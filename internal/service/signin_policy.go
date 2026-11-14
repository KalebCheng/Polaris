package service

import (
	"context"
	"strings"

	"polaris/internal/logger"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// SigninPolicyResolver 是“组织/应用最终有哪些登录方式”的唯一运行时入口。
//
// 应用只保存选择关系；方法参数、提供商、模板与 MFA 均由组织方法提供。
type SigninPolicyResolver struct {
	methods   repository.InstitutionSigninMethodRepository
	providers repository.ProviderRepository
	templates *TemplateService
}

// NewSigninPolicyResolver 创建登录方式运行时解析器。
func NewSigninPolicyResolver(
	methods repository.InstitutionSigninMethodRepository,
	providers repository.ProviderRepository,
	templates *TemplateService,
) *SigninPolicyResolver {
	return &SigninPolicyResolver{methods: methods, providers: providers, templates: templates}
}

// OrganizationMethods 返回组织全部启用且可用的登录方式。
func (r *SigninPolicyResolver) OrganizationMethods(ctx context.Context, institutionID int64) ([]repository.SigninMethod, error) {
	if r == nil || r.methods == nil {
		return []repository.SigninMethod{}, nil
	}
	items, err := r.methods.ListActive(ctx, institutionID)
	if err != nil {
		return nil, err
	}
	return r.effectiveMethods(ctx, institutionID, items), nil
}

// ApplicationMethods 返回组织启用方法与应用选择的交集。
//
// app == nil 表示平台登录，等同于 OrganizationMethods。
func (r *SigninPolicyResolver) ApplicationMethods(ctx context.Context, app *repository.Application) ([]repository.SigninMethod, error) {
	if app == nil {
		return []repository.SigninMethod{}, nil
	}
	return r.ApplicationMethodsByID(ctx, app.InstitutionID, app.ID)
}

// Methods 按“平台登录 / 应用登录”选择正确的策略来源。
func (r *SigninPolicyResolver) Methods(ctx context.Context, institutionID int64, app *repository.Application) ([]repository.SigninMethod, error) {
	if app != nil {
		return r.ApplicationMethods(ctx, app)
	}
	return r.OrganizationMethods(ctx, institutionID)
}

// ApplicationMethodsByID 按应用主键解析，供只需要 appID 的调用方使用。
func (r *SigninPolicyResolver) ApplicationMethodsByID(ctx context.Context, institutionID, applicationID int64) ([]repository.SigninMethod, error) {
	if r == nil || r.methods == nil {
		return []repository.SigninMethod{}, nil
	}
	items, err := r.methods.ListByApplication(ctx, institutionID, applicationID)
	if err != nil {
		return nil, err
	}
	selected := make([]*repository.InstitutionSigninMethod, 0, len(items))
	for _, item := range items {
		if item.IsActive() {
			selected = append(selected, item)
		}
	}
	return r.effectiveMethods(ctx, institutionID, selected), nil
}

// FindApplicationMethod 在应用 effective methods 中按 kind 查找。
func (r *SigninPolicyResolver) FindApplicationMethod(ctx context.Context, app *repository.Application, name string) (repository.SigninMethod, bool, error) {
	methods, err := r.ApplicationMethods(ctx, app)
	if err != nil {
		return repository.SigninMethod{}, false, err
	}
	return findMethod(methods, name)
}

// FindOrganizationMethod 在组织 effective methods 中按 kind 查找。
func (r *SigninPolicyResolver) FindOrganizationMethod(ctx context.Context, institutionID int64, name string) (repository.SigninMethod, bool, error) {
	methods, err := r.OrganizationMethods(ctx, institutionID)
	if err != nil {
		return repository.SigninMethod{}, false, err
	}
	return findMethod(methods, name)
}

// ProviderItems 把 effective Provider 方法转换为登录页的提供商白名单。
func (r *SigninPolicyResolver) ProviderItems(ctx context.Context, methods []repository.SigninMethod) []repository.ProviderItem {
	if r == nil || r.providers == nil {
		return []repository.ProviderItem{}
	}
	items := make([]repository.ProviderItem, 0)
	seen := map[int64]bool{}
	for _, method := range methods {
		if method.Name != repository.SigninMethodProvider || method.ProviderID <= 0 || seen[method.ProviderID] {
			continue
		}
		provider, err := r.providers.GetByID(ctx, method.ProviderID)
		if err != nil || provider == nil || !strings.EqualFold(provider.State, "active") {
			continue
		}
		if providerpkg.NormalizeCategory(provider.Category) != string(providerpkg.CategoryOAuth) {
			continue
		}
		seen[method.ProviderID] = true
		items = append(items, repository.ProviderItem{
			Name:      provider.Name,
			CanSignIn: true,
		})
	}
	return items
}

func (r *SigninPolicyResolver) effectiveMethods(ctx context.Context, institutionID int64, items []*repository.InstitutionSigninMethod) []repository.SigninMethod {
	out := make([]repository.SigninMethod, 0, len(items))
	for _, item := range items {
		method, ok := r.effectiveMethod(ctx, institutionID, item)
		if !ok {
			continue
		}
		out = append(out, method)
	}
	return out
}

func (r *SigninPolicyResolver) effectiveMethod(ctx context.Context, institutionID int64, item *repository.InstitutionSigninMethod) (repository.SigninMethod, bool) {
	if item == nil || item.InstitutionID != institutionID || !item.IsActive() {
		return repository.SigninMethod{}, false
	}
	switch item.Name {
	case repository.SigninMethodPassword:
		if len(item.Identifiers) == 0 {
			r.warn(institutionID, item, "password identifiers are empty")
			return repository.SigninMethod{}, false
		}
	case repository.SigninMethodEmailCode, repository.SigninMethodMagicLink:
		if item.ProviderID == nil || item.EmailTemplateID == nil {
			r.warn(institutionID, item, "email provider/template is missing")
			return repository.SigninMethod{}, false
		}
		scene := repository.TemplateSceneLoginCode
		if item.Name == repository.SigninMethodMagicLink {
			scene = repository.TemplateSceneMagicLink
		}
		if r.templates == nil {
			r.warn(institutionID, item, "template service is not configured")
			return repository.SigninMethod{}, false
		}
		_, tpl, err := r.templates.ResolveNotification(ctx, institutionID, *item.ProviderID, *item.EmailTemplateID,
			scene, providerpkg.CategoryEmail)
		if err != nil || tpl == nil {
			r.warn(institutionID, item, "email provider/template is unavailable")
			return repository.SigninMethod{}, false
		}
	case repository.SigninMethodSMSCode:
		if item.ProviderID == nil || item.SMSTemplateID == nil {
			r.warn(institutionID, item, "sms provider/template is missing")
			return repository.SigninMethod{}, false
		}
		if r.templates == nil {
			r.warn(institutionID, item, "template service is not configured")
			return repository.SigninMethod{}, false
		}
		_, tpl, err := r.templates.ResolveNotification(ctx, institutionID, *item.ProviderID, *item.SMSTemplateID,
			repository.TemplateSceneLoginCode, providerpkg.CategorySMS)
		if err != nil || tpl == nil {
			r.warn(institutionID, item, "sms provider/template is unavailable")
			return repository.SigninMethod{}, false
		}
	case repository.SigninMethodProvider:
		if item.ProviderID == nil {
			r.warn(institutionID, item, "oauth provider is missing")
			return repository.SigninMethod{}, false
		}
		provider, err := r.providers.GetByID(ctx, *item.ProviderID)
		if err != nil || provider == nil || provider.InstitutionID != institutionID ||
			!strings.EqualFold(provider.State, "active") ||
			providerpkg.NormalizeCategory(provider.Category) != string(providerpkg.CategoryOAuth) {
			r.warn(institutionID, item, "oauth provider is unavailable")
			return repository.SigninMethod{}, false
		}
	default:
		r.warn(institutionID, item, "unsupported sign-in method")
		return repository.SigninMethod{}, false
	}

	out := repository.EffectiveSigninMethod(item)
	if out.DisplayName == "" {
		out.DisplayName = defaultMethodDisplayName(item, r.providersForName(ctx, institutionID, item))
	}
	out.MfaProviderIDs = r.validMFAProviderIDs(ctx, institutionID, item.MfaProviderIDs)
	if item.MfaEnabled && len(out.MfaProviderIDs) == 0 {
		out.MfaProviderIDs = r.defaultMFAProviderIDs(ctx, institutionID)
	}
	if item.MfaEnabled && len(out.MfaProviderIDs) == 0 {
		// 不把“MFA 配了但没有可用提供商”变成静默降级或整条方法消失：
		// 保持方法可用但显式关闭 MFA，并留下告警，避免把用户锁在门外。
		r.warn(institutionID, item, "mfa enabled but no active mfa provider; mfa is disabled")
		out.MfaEnabled = false
		out.MfaRequired = false
	}
	return out, true
}

func (r *SigninPolicyResolver) providersForName(ctx context.Context, institutionID int64, item *repository.InstitutionSigninMethod) *repository.Provider {
	if item.ProviderID == nil || r.providers == nil {
		return nil
	}
	provider, err := r.providers.GetByID(ctx, *item.ProviderID)
	if err != nil || provider == nil || provider.InstitutionID != institutionID {
		return nil
	}
	return provider
}

func defaultMethodDisplayName(item *repository.InstitutionSigninMethod, provider *repository.Provider) string {
	if item == nil {
		return ""
	}
	if item.Name == repository.SigninMethodProvider && provider != nil {
		return firstNonEmptyStr(provider.DisplayName, provider.Name)
	}
	switch item.Name {
	case repository.SigninMethodPassword:
		return "Password"
	case repository.SigninMethodEmailCode:
		return "Email code"
	case repository.SigninMethodSMSCode:
		return "SMS code"
	case repository.SigninMethodMagicLink:
		return "Magic link"
	default:
		return item.Name
	}
}

func (r *SigninPolicyResolver) validMFAProviderIDs(ctx context.Context, institutionID int64, ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	if r.providers == nil {
		return out
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		provider, err := r.providers.GetByID(ctx, id)
		if err != nil || provider == nil || provider.InstitutionID != institutionID ||
			!strings.EqualFold(provider.State, "active") ||
			providerpkg.NormalizeCategory(provider.Category) != string(providerpkg.CategoryMFA) {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (r *SigninPolicyResolver) defaultMFAProviderIDs(ctx context.Context, institutionID int64) []int64 {
	if r.providers == nil {
		return []int64{}
	}
	items, err := r.providers.List(ctx, repository.ListQuery{
		InstitutionID: institutionID,
		Field:         "category",
		Value:         string(providerpkg.CategoryMFA),
		Limit:         1000,
	})
	if err != nil {
		return []int64{}
	}
	for _, item := range items {
		if item.Name == "default-totp" && strings.EqualFold(item.State, "active") {
			return []int64{item.ID}
		}
	}
	for _, item := range items {
		if strings.EqualFold(item.State, "active") &&
			providerpkg.NormalizeType(providerpkg.CategoryMFA, item.Type) == providerpkg.TypeMFATOTP {
			return []int64{item.ID}
		}
	}
	return []int64{}
}

func findMethod(methods []repository.SigninMethod, name string) (repository.SigninMethod, bool, error) {
	for _, method := range methods {
		if method.Name == name {
			return method, true, nil
		}
	}
	return repository.SigninMethod{}, false, nil
}

func (r *SigninPolicyResolver) warn(institutionID int64, item *repository.InstitutionSigninMethod, reason string) {
	methodID := int64(0)
	name := ""
	if item != nil {
		methodID = item.ID
		name = item.Name
	}
	logger.Warn("sign-in method skipped",
		"institution_id", institutionID, "method_id", methodID, "method", name, "reason", reason)
}
