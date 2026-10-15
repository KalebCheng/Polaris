package service

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// applicationCodePattern 应用标识（code）的合法形式：
// 小写字母或数字开头，允许 - 与 _，长度 2–31。禁止中文、大写与空格
// （标识会进入 URL 与外部集成，创建后不可修改）。
var applicationCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-_]{1,30}$`)

// ApplicationService 应用服务
type ApplicationService struct {
	apps   repository.ApplicationRepository
	orgs   repository.InstitutionRepository
	audits *AuditRecorder
}

// NewApplicationService 创建应用服务
func NewApplicationService(apps repository.ApplicationRepository, orgs repository.InstitutionRepository, audits *AuditRecorder) *ApplicationService {
	return &ApplicationService{apps: apps, orgs: orgs, audits: audits}
}

// Create 创建应用：校验标识、补全默认值、并由服务端生成客户端凭据
// （client_id / client_secret 一律不接受调用方传入）。
func (s *ApplicationService) Create(ctx context.Context, actor string, app *repository.Application) (*repository.Application, error) {
	app.Code = strings.TrimSpace(app.Code)
	if app.InstitutionID == 0 || app.Code == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institutionID and code are required")
	}
	if !applicationCodePattern.MatchString(app.Code) {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams,
			"code must be 2-31 characters of lowercase letters, digits, '-' or '_'")
	}
	if _, err := s.orgs.GetByID(ctx, app.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", app.InstitutionID)
	}
	if _, err := s.apps.GetByCode(ctx, app.InstitutionID, app.Code); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "application already exists")
	}
	// 凭据由服务端生成，忽略请求体中的值
	app.ClientID = randomHex(10)
	app.ClientSecret = randomBase64(32)
	if err := normalizeApplication(app); err != nil {
		return nil, err
	}
	id, err := s.apps.Create(ctx, app)
	if err != nil {
		return nil, err
	}
	app.ID = id
	s.audits.Record(ctx, actor, app.InstitutionID, "application.create", app.Code, nil)
	return app, nil
}

// Get 查询应用
func (s *ApplicationService) Get(ctx context.Context, institutionID int64, code string) (*repository.Application, error) {
	app, err := s.apps.GetByCode(ctx, institutionID, code)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("application", code)
	}
	return app, nil
}

// Update 更新应用（code / client_id / client_secret 均为不可变标识）。
func (s *ApplicationService) Update(ctx context.Context, actor string, app *repository.Application) error {
	existing, err := s.apps.GetByCode(ctx, app.InstitutionID, app.Code)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("application", app.Code)
	}
	app.ID = existing.ID
	app.ClientID = existing.ClientID
	app.ClientSecret = existing.ClientSecret
	// 列表排序值：表单不承载该字段，避免更新时被零值静默重置
	if app.Order == 0 {
		app.Order = existing.Order
	}
	if strings.TrimSpace(app.Name) == "" {
		app.Name = existing.Name
	}
	if err := normalizeApplication(app); err != nil {
		return err
	}
	if err := s.apps.Update(ctx, app); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, app.InstitutionID, "application.update", app.Code, nil)
	return nil
}

// Delete 删除应用
func (s *ApplicationService) Delete(ctx context.Context, actor string, institutionID int64, code string) error {
	app, err := s.apps.GetByCode(ctx, institutionID, code)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("application", code)
	}
	if err := s.apps.Delete(ctx, app.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "application.delete", code, nil)
	return nil
}

// List 分页列出应用
func (s *ApplicationService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Application, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.apps.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.apps.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ResetSecret 重新生成应用密钥（响应仅在本次返回明文）
func (s *ApplicationService) ResetSecret(ctx context.Context, actor string, institutionID int64, code string) (string, error) {
	app, err := s.apps.GetByCode(ctx, institutionID, code)
	if err != nil {
		return "", bizerrors.ErrNotFound.WithDetail("application", code)
	}
	app.ClientSecret = randomBase64(32)
	if err := s.apps.Update(ctx, app); err != nil {
		return "", err
	}
	s.audits.Record(ctx, actor, institutionID, "application.reset_secret", code, nil)
	return app.ClientSecret, nil
}

// normalizeApplication 补全默认值并执行跨字段校验。
// 「配置即生效」要求：登录方式不得为空（否则登录页无任何入口）；启用授权码模式必须有回调地址。
func normalizeApplication(app *repository.Application) error {
	if strings.TrimSpace(app.Name) == "" {
		app.Name = app.Code
	}
	if app.Category == "" {
		app.Category = "default"
	}
	if app.Type == "" {
		app.Type = "oidc"
	}
	if len(app.SigninMethods) == 0 {
		app.SigninMethods = []repository.SigninMethod{
			{Name: repository.SigninMethodPassword, DisplayName: "Password", Rule: "All"},
		}
	}
	if len(app.GrantTypes) == 0 {
		app.GrantTypes = []string{"authorization_code", "password", "refresh_token", "client_credentials"}
	}
	if app.TokenFormat == "" {
		app.TokenFormat = "jwt"
	}
	if app.TokenSigningMethod == "" {
		app.TokenSigningMethod = "RS256"
	}
	if app.ExpireInHours == 0 {
		app.ExpireInHours = 1
	}
	if app.RefreshExpireInHours == 0 {
		app.RefreshExpireInHours = 24
	}
	if app.CookieExpireInHours == 0 {
		app.CookieExpireInHours = 720
	}
	if app.State == "" {
		app.State = "active"
	}
	if app.Adapter != "custom" {
		app.Adapter = "default"
		app.AdapterConfig = nil
	}
	if app.EmailCodeTTLSeconds <= 0 {
		app.EmailCodeTTLSeconds = 300
	}
	if app.SMSCodeTTLSeconds <= 0 {
		app.SMSCodeTTLSeconds = 300
	}
	if app.MagicLinkTTLSeconds <= 0 {
		app.MagicLinkTTLSeconds = 600
	}
	return validateApplication(app)
}

// validateApplication 校验跨字段约束（服务层统一出口，错误码复用通用非法参数码）。
func validateApplication(app *repository.Application) error {
	if len(app.SigninMethods) == 0 {
		return bizerrors.New(bizerrors.CodeInvalidParams, "at least one sign-in method is required")
	}
	// 登录方式必须是"已实现运行时"的方式，且第三方登录必须带具体提供方
	// （前端按"一个提供方一张卡片"提交，接口层同样不允许空名或未实现的方式混进来）
	for _, method := range app.SigninMethods {
		if !implementedSigninMethods[method.Name] {
			return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported sign-in method").
				WithDetail("method", method.Name)
		}
		if method.Name != repository.SigninMethodProvider {
			continue
		}
		named := 0
		for _, provider := range app.Providers {
			if strings.TrimSpace(provider.Name) != "" {
				named++
			}
		}
		if named == 0 {
			return bizerrors.New(bizerrors.CodeInvalidParams,
				"provider sign-in requires at least one provider")
		}
	}
	if app.SigninErrorLimit < 0 || app.SigninRetryWaitMinutes < 0 {
		return bizerrors.New(bizerrors.CodeInvalidParams,
			"signin_error_limit and signin_retry_wait_minutes must not be negative")
	}
	if contains(app.GrantTypes, "authorization_code") && len(app.RedirectURIs) == 0 {
		return bizerrors.New(bizerrors.CodeInvalidParams,
			"redirect_uris is required when authorization_code grant is enabled")
	}
	for _, uri := range app.RedirectURIs {
		if !validRedirectURI(uri) {
			return bizerrors.New(bizerrors.CodeInvalidParams, "invalid redirect_uri").
				WithDetail("redirect_uri", uri)
		}
	}
	return nil
}

// validRedirectURI 校验回调地址：必须 https（本机地址允许 http），不得带 fragment，必须有主机名。
func validRedirectURI(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	// 允许通配子路径写法（与 appAllowsRedirect 保持一致）：https://app.example.com/*
	probe := strings.TrimSuffix(raw, "*")
	u, err := url.Parse(probe)
	if err != nil || u.Host == "" || u.Fragment != "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	default:
		return false
	}
}

// CodeByID 按主键解析应用标识 code（对外 HTTP 路径统一用 id；内部仍以 code 语义访问存储）。
func (s *ApplicationService) CodeByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.apps.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Code, nil
}
