package service

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"polaris/internal/auth/tokenhash"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// applicationCodePattern 应用标识（code）的合法形式：
// 小写字母或数字开头，允许 - 与 _，长度 2–31。禁止中文、大写与空格
// （标识会进入 URL 与外部集成，创建后不可修改）。
var applicationCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-_]{1,30}$`)

// ApplicationService 应用服务
type ApplicationService struct {
	apps      repository.ApplicationRepository
	orgs      repository.InstitutionRepository
	providers repository.ProviderRepository
	templates *TemplateService
	audits    *AuditRecorder
}

// NewApplicationService 创建应用服务
func NewApplicationService(apps repository.ApplicationRepository, orgs repository.InstitutionRepository, audits *AuditRecorder) *ApplicationService {
	return &ApplicationService{apps: apps, orgs: orgs, audits: audits}
}

// NewApplicationServiceWithResources constructs the production service in one step.
func NewApplicationServiceWithResources(
	apps repository.ApplicationRepository,
	orgs repository.InstitutionRepository,
	providers repository.ProviderRepository,
	templates *TemplateService,
	audits *AuditRecorder,
) *ApplicationService {
	return &ApplicationService{apps: apps, orgs: orgs, providers: providers, templates: templates, audits: audits}
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
	app.ClientSecretPlain = randomBase64(32)
	app.ClientSecretHash = tokenhash.Hash(app.ClientSecretPlain)
	// 访问模式：新建一律"全员可访问"（与引入该能力之前的行为一致）。
	// 受限模式只能由 PUT /applications/:id/access 打开——它是"模式 + 名单"的原子写入口。
	app.AccessMode = repository.AccessModeAll
	if err := normalizeApplication(app); err != nil {
		return nil, err
	}
	if err := s.normalizeSigninMethods(ctx, app, true); err != nil {
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
	app.ClientSecretHash = existing.ClientSecretHash
	app.ClientSecretPlain = ""
	// 列表排序值：表单不承载该字段，避免更新时被零值静默重置
	if app.Order == 0 {
		app.Order = existing.Order
	}
	// 访问模式：**恒从现有值继承**，唯一写入口是 PUT /applications/:id/access。
	// 反例（历史教训型陷阱）：本端点是整行 UPDATE，若让零值 "" 落库会违反 access_mode 的 CHECK 约束
	// 抛 DB_ERROR；若在仓库层把 "" 兜底成 all，则会**静默把受限应用改成全员可访问**（放宽准入）。
	app.AccessMode = existing.AccessMode
	if strings.TrimSpace(app.Name) == "" {
		app.Name = existing.Name
	}
	if err := normalizeApplication(app); err != nil {
		return err
	}
	if err := s.normalizeSigninMethods(ctx, app, false); err != nil {
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
	app.ClientSecretPlain = randomBase64(32)
	app.ClientSecretHash = tokenhash.Hash(app.ClientSecretPlain)
	if err := s.apps.Update(ctx, app); err != nil {
		return "", err
	}
	s.audits.Record(ctx, actor, institutionID, "application.reset_secret", code, nil)
	return app.ClientSecretPlain, nil
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
	if app.TokenFormat == "" || app.TokenFormat == "jwt" {
		app.TokenFormat = TokenFormatCustom
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

// normalizeSigninMethods 补齐登录方式默认值并校验显式资源引用。
// 这是“保存时失败”而不是“运行时才发现”的唯一入口。
func (s *ApplicationService) normalizeSigninMethods(ctx context.Context, app *repository.Application, create bool) error {
	for i := range app.SigninMethods {
		method := &app.SigninMethods[i]
		switch method.Name {
		case repository.SigninMethodPassword:
			if method.Identifiers == nil {
				if create {
					method.Identifiers = []string{repository.PasswordIdentifierUsername}
				} else {
					method.Identifiers = []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail}
				}
			}
			identifiers := make([]string, 0, len(method.Identifiers))
			seen := map[string]bool{}
			for _, kind := range method.Identifiers {
				kind = strings.ToLower(strings.TrimSpace(kind))
				if !repository.IsPasswordIdentifier(kind) {
					return bizerrors.ErrInvalidParams.WithDetail("identifier", kind)
				}
				if !seen[kind] {
					seen[kind] = true
					identifiers = append(identifiers, kind)
				}
			}
			if len(identifiers) == 0 {
				return bizerrors.ErrInvalidParams.WithDetail("required", "identifiers")
			}
			ordered := make([]string, 0, len(identifiers))
			for _, kind := range []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail, repository.PasswordIdentifierPhone} {
				if seen[kind] {
					ordered = append(ordered, kind)
				}
			}
			method.Identifiers = ordered
		case repository.SigninMethodEmailCode, repository.SigninMethodSMSCode, repository.SigninMethodMagicLink:
			if err := s.normalizeNotificationMethod(ctx, app.InstitutionID, method); err != nil {
				return err
			}
		}
		method.MfaEnabled = method.MFAEnabled()
		method.MfaRequired = method.MFARequired()
		if method.MfaRequired && !method.MfaEnabled {
			return bizerrors.ErrInvalidParams.WithDetail("required", "mfa_enabled")
		}
		if method.MfaEnabled {
			ids, err := s.normalizeMFAProviders(ctx, app.InstitutionID, method.MfaProviderIDs)
			if err != nil {
				return err
			}
			method.MfaProviderIDs = ids
		} else {
			method.MfaProviderIDs = []int64{}
			method.MfaRequired = false
		}
	}
	return validateApplication(app)
}

func (s *ApplicationService) normalizeNotificationMethod(ctx context.Context, institutionID int64, method *repository.SigninMethod) error {
	expected := providerpkg.CategoryEmail
	scene := repository.TemplateSceneLoginCode
	if method.Name == repository.SigninMethodSMSCode {
		expected = providerpkg.CategorySMS
	}
	if method.Name == repository.SigninMethodMagicLink {
		scene = repository.TemplateSceneMagicLink
	}
	if s.providers == nil || s.templates == nil {
		return nil
	}
	provider, err := s.providers.GetByID(ctx, method.ProviderID)
	if err != nil || provider.InstitutionID != institutionID ||
		providerpkg.NormalizeCategory(provider.Category) != string(expected) ||
		!strings.EqualFold(provider.State, "active") {
		return bizerrors.ErrInvalidParams.WithDetail("provider_id", method.ProviderID).WithDetail("method", method.Name)
	}
	_, tpl, err := s.templates.ResolveNotification(ctx, institutionID, method.ProviderID, method.TemplateID, scene, expected)
	if err != nil {
		return err
	}
	method.TemplateID = tpl.ID
	return nil
}

func (s *ApplicationService) normalizeMFAProviders(ctx context.Context, institutionID int64, ids []int64) ([]int64, error) {
	ids = dedupeInt64(ids)
	if s.providers == nil {
		return ids, nil
	}
	if len(ids) == 0 {
		items, err := s.providers.List(ctx, repository.ListQuery{
			InstitutionID: institutionID, Field: "category", Value: string(providerpkg.CategoryMFA), Limit: 1000,
		})
		if err != nil {
			return nil, err
		}
		var fallback int64
		for _, item := range items {
			if !strings.EqualFold(item.State, "active") {
				continue
			}
			if item.Name == "default-totp" {
				fallback = item.ID
				break
			}
			if fallback == 0 && providerpkg.NormalizeType(providerpkg.CategoryMFA, item.Type) == providerpkg.TypeMFATOTP {
				fallback = item.ID
			}
		}
		if fallback == 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "mfa_provider_ids")
		}
		return []int64{fallback}, nil
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		item, err := s.providers.GetByID(ctx, id)
		if err != nil || item.InstitutionID != institutionID ||
			providerpkg.NormalizeCategory(item.Category) != string(providerpkg.CategoryMFA) ||
			!strings.EqualFold(item.State, "active") {
			return nil, bizerrors.ErrInvalidParams.WithDetail("mfa_provider_id", id)
		}
		out = append(out, id)
	}
	return out, nil
}

// validateApplication 校验跨字段约束（服务层统一出口，错误码复用通用非法参数码）。
func validateApplication(app *repository.Application) error {
	switch app.TokenFormat {
	case "", "jwt", TokenFormatCustom:
		app.TokenFormat = TokenFormatCustom
	case TokenFormatStandard:
	default:
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported token_format").
			WithDetail("token_format", app.TokenFormat)
	}
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
	attributes, err := normalizeTokenAttributes(app.TokenAttributes)
	if err != nil {
		return err
	}
	app.TokenAttributes = attributes
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
