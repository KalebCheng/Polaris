package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// 本文件实现终端用户自助注册（托管注册页 /signup 的后端入口）。
//
// 「配置即生效」：
//   - 是否允许注册由应用的 self_signup_enabled 唯一决定；
//   - 表单字段与必填/正则由应用的 signup_items 编排决定，未声明的字段一律拒绝；
//   - 注册成功后是否自动登录由应用的 signup_auto_signin 决定。

const (
	signupRateScope  = "polaris:ratelimit:signup:"
	signupHourlyMax  = 20
	signupRateWindow = time.Hour
)

// SignupRequest 自助注册请求（托管注册页按 signup_items 编排提交）。
type SignupRequest struct {
	// Institution 组织标识（兼容）；InstitutionID 组织主键（权威）
	Institution   string `json:"institution"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application"`
	ClientID      string `json:"client_id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Password      string `json:"password"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	From          string `json:"from"`
	// Fields 承载编排中的其他字段（如 "First name" / "Terms of use"）。
	Fields map[string]string `json:"fields"`
}

// Signup 自助注册：返回创建的用户；当应用开启自动登录时同时返回会话 ID。
func (s *AuthService) Signup(ctx context.Context, req SignupRequest, ip, userAgent string) (*repository.User, *auth.Session, error) {
	institution := strings.TrimSpace(req.Institution)
	if req.InstitutionID != 0 {
		// 公开面以主键为权威引用；解析失败直接报错，不静默回落到默认组织
		org, err := s.orgs.GetByID(ctx, req.InstitutionID)
		if err != nil || org == nil {
			return nil, nil, bizerrors.ErrNotFound.WithDetail("institution_id", req.InstitutionID)
		}
		institution = org.Name
	}
	if institution == "" {
		institution = s.oauthCfg.DefaultInstitution
	}
	if institution == "" {
		return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution is required")
	}
	org, err := s.orgs.GetByName(ctx, institution)
	if err != nil {
		return nil, nil, bizerrors.ErrNotFound.WithDetail("institution", institution)
	}
	if org.DisableSignin {
		return nil, nil, bizerrors.New(bizerrors.CodeForbidden, "sign-up is disabled for this institution")
	}
	// 应用可选：组织未配置应用时不阻断自助注册（宽松解析返回 nil，跳过应用级开关）
	app, err := s.resolveSignupApp(ctx, institution, req)
	if err != nil {
		return nil, nil, err
	}
	if app != nil && !app.SelfSignupEnabled {
		return nil, nil, bizerrors.New(bizerrors.CodeForbidden, "self sign-up is disabled for this application")
	}

	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(req.Email)
	password := req.Password
	if name == "" || password == "" {
		return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "name and password are required")
	}
	values := signupValues(req)
	if app != nil {
		if err := validateSignupFields(app.SignupItems, values, name, email, password); err != nil {
			return nil, nil, err
		}
	}
	// 应用准入：受限应用下自助注册一律拒绝——新用户不可能出现在授权名单里，
	// 放进来只会得到"注册成功但登不进去"的死路。仅当请求显式携带应用上下文时判定（Spec FR-3）。
	if s.access != nil {
		appCtx, cerr := s.resolveAppContext(ctx, org.ID, institution, req.Application, req.ClientID)
		if cerr != nil {
			return nil, nil, cerr
		}
		if appCtx != nil && appCtx.AccessMode == repository.AccessModeRestricted {
			s.audits.Record(ctx, name, org.ID, "auth.app_access_denied", appCtx.Code,
				map[string]interface{}{"flow": "signup"})
			return nil, nil, bizerrors.NewWithCode(bizerrors.CodeAppAccessDenied).WithDetail("application", appCtx.Code)
		}
	}

	// 注册频率限制（按组织 + IP）
	if s.rateLimiter != nil {
		key := signupRateScope + org.Name + ":" + ip
		if n, _ := s.rateLimiter.Get(ctx, key); n >= signupHourlyMax {
			return nil, nil, bizerrors.ErrVerifyCodeTooMany.WithDetail("window", "hourly")
		}
		if _, incrErr := s.rateLimiter.Incr(ctx, key, signupRateWindow); incrErr != nil {
			return nil, nil, incrErr
		}
	}

	user := &repository.User{
		InstitutionID:  org.ID,
		Name:           name,
		DisplayName:    strings.TrimSpace(req.DisplayName),
		Email:          email,
		Phone:          strings.TrimSpace(req.Phone),
		RegisterType:   "password",
		RegisterSource: "signup",
		CreatedIP:      ip,
	}
	if app != nil {
		user.SignupApplication = app.Code
	}
	created, err := s.userService.Create(ctx, name, CreateUserInput{User: user, Password: password})
	if err != nil {
		return nil, nil, err
	}
	s.audits.Record(ctx, name, org.ID, "user.signup", name, map[string]interface{}{
		"application": applicationCode(app),
	})
	if app != nil {
		s.addDefaultJobTitle(ctx, org.ID, app.DefaultGroup, created.Name)
	}

	// 自动登录（应用级开关，默认开启）
	if app == nil || !app.SignupAutoSignin {
		return created, nil, nil
	}
	session, err := s.sessions.Create(ctx, created, app.Code, ip, userAgent)
	if err != nil {
		return created, nil, nil // 注册已成功，自动登录失败不阻断
	}
	s.enforceSessionPolicy(ctx, app, created, session.ID)
	if s.provisioning != nil {
		s.provisioning.TriggerJIT(ctx, created.InstitutionID, app.Code, created.Name)
	}
	return created, session, nil
}

// LoginResponse 依据已建立的会话生成登录响应（供注册后自动登录复用）。
func (s *AuthService) LoginResponse(user *repository.User, session *auth.Session, from string) *auth.LoginResponse {
	if user == nil || session == nil {
		return nil
	}
	return s.loginResponse(user, session, from)
}

// resolveSignupApp 解析注册所属应用：优先 client_id，其次 institutionID+application。
func (s *AuthService) resolveSignupApp(ctx context.Context, institution string, req SignupRequest) (*repository.Application, error) {
	if clientID := strings.TrimSpace(req.ClientID); clientID != "" {
		app, err := s.apps.GetByClientID(ctx, clientID)
		if err != nil {
			return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("client_id", clientID)
		}
		return app, nil
	}
	return s.resolveApplicationOptional(ctx, institution, req.Application)
}

// signupValues 把请求归一化为"字段名(小写) → 值"的映射，供编排校验使用。
func signupValues(req SignupRequest) map[string]string {
	values := map[string]string{
		"username":     strings.TrimSpace(req.Name),
		"display name": strings.TrimSpace(req.DisplayName),
		"email":        strings.TrimSpace(req.Email),
		"phone":        strings.TrimSpace(req.Phone),
		"password":     req.Password,
	}
	for k, v := range req.Fields {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		values[key] = v
	}
	return values
}

// validateSignupFields 依据应用注册字段编排校验：
//   - 未在编排中声明的字段一律拒绝（防越权注入）；
//   - 编排中 visible 且 required 的字段必须存在；
//   - 声明了 regex 的字段必须匹配。
//
// 编排为空时跳过校验（兼容未配置编排的历史应用）。
func validateSignupFields(items []repository.SignupItem, values map[string]string, name, email, password string) error {
	if len(items) == 0 {
		return nil
	}
	declared := map[string]repository.SignupItem{}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" {
			continue
		}
		declared[key] = item
	}
	// 兜底：核心字段视为已声明（它们始终可提交）
	declared["username"] = repository.SignupItem{Name: "Username", Visible: true, Required: true}
	declared["password"] = repository.SignupItem{Name: "Password", Visible: true, Required: true}
	if email != "" {
		declared["email"] = repository.SignupItem{Name: "Email", Visible: true, Required: false}
	}

	for field, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := declared[field]; !ok {
			return bizerrors.New(bizerrors.CodeInvalidParams,
				"field is not allowed by the signup form").WithDetail("field", field)
		}
	}
	for key, item := range declared {
		if !item.Visible || !item.Required {
			continue
		}
		value := values[key]
		if strings.TrimSpace(value) == "" {
			return bizerrors.New(bizerrors.CodeInvalidParams,
				"required field is missing").WithDetail("field", item.Name)
		}
		if item.Regex != "" {
			re, err := regexp.Compile(item.Regex)
			if err == nil && !re.MatchString(value) {
				return bizerrors.New(bizerrors.CodeInvalidParams,
					"field does not match the required format").WithDetail("field", item.Name)
			}
		}
	}
	return nil
}
