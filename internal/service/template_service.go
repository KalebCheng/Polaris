package service

import (
	"context"
	"regexp"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// TemplateService 通知模板服务：模板 CRUD、场景校验、默认模板与引用完整性。
//
// 运行时业务层必须经 ResolveNotification 解析 provider + template，禁止直接读
// providers.template_code/title/content（这些列仅保留给迁移回滚与旧数据）。
type TemplateService struct {
	templates repository.ProviderTemplateRepository
	providers repository.ProviderRepository
	apps      repository.ApplicationRepository
	runtime   *providerpkg.Runtime
	audits    *AuditRecorder
}

// NewTemplateService 创建通知模板服务。
func NewTemplateService(
	templates repository.ProviderTemplateRepository,
	providers repository.ProviderRepository,
	apps repository.ApplicationRepository,
	runtime *providerpkg.Runtime,
	audits *AuditRecorder,
) *TemplateService {
	return &TemplateService{templates: templates, providers: providers, apps: apps, runtime: runtime, audits: audits}
}

// NotificationTemplateInput 模板写入契约。
type NotificationTemplateInput struct {
	Name           string
	Scene          string
	TemplateCode   string
	TemplateParams []string
	Subject        string
	Content        string
	IsDefault      bool
}

// List 列出某提供商的模板（可按 scene 过滤）。
func (s *TemplateService) List(ctx context.Context, institutionID, providerID int64, scene string) ([]*repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, err
	}
	if scene != "" && !repository.IsTemplateScene(scene) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("scene", scene)
	}
	return s.templates.ListByProvider(ctx, channelForCategory(provider.Category), institutionID, providerID, scene)
}

// Get 按 provider 主键 + 模板主键查询（渠道由 provider 类别决定）。
func (s *TemplateService) Get(ctx context.Context, institutionID, providerID, templateID int64) (*repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, err
	}
	item, err := s.templates.GetByID(ctx, channelForCategory(provider.Category), templateID)
	if err != nil || item.InstitutionID != institutionID || item.ProviderID != providerID {
		return nil, bizerrors.ErrNotFound.WithDetail("template_id", templateID)
	}
	return item, nil
}

// Create 新建模板。
func (s *TemplateService) Create(ctx context.Context, actor string, institutionID, providerID int64, in NotificationTemplateInput) (*repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeTemplateInput(provider.Category, in)
	if err != nil {
		return nil, err
	}
	if normalized.IsDefault {
		if err := s.clearDefault(ctx, channelForCategory(provider.Category), institutionID, providerID, normalized.Scene, 0); err != nil {
			return nil, err
		}
	}
	item := &repository.ProviderTemplate{
		InstitutionID: institutionID, ProviderID: providerID,
		Channel: channelForCategory(provider.Category),
		Name:    normalized.Name, Scene: normalized.Scene,
		TemplateCode: normalized.TemplateCode, TemplateParams: normalized.TemplateParams,
		Subject: normalized.Subject, Content: normalized.Content, IsDefault: normalized.IsDefault,
	}
	id, err := s.templates.Create(ctx, item)
	if err != nil {
		return nil, err
	}
	item.ID = id
	s.audits.Record(ctx, actor, institutionID, "provider_template.create", item.Name, map[string]interface{}{"provider_id": providerID, "scene": item.Scene})
	return item, nil
}

// Update 更新模板（模板必须属于 URL 中指定的提供商）。
func (s *TemplateService) Update(ctx context.Context, actor string, institutionID, providerID, templateID int64, in NotificationTemplateInput) (*repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, institutionID, providerID, templateID)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeTemplateInput(provider.Category, in)
	if err != nil {
		return nil, err
	}
	if normalized.IsDefault {
		if err := s.clearDefault(ctx, existing.Channel, institutionID, providerID, normalized.Scene, templateID); err != nil {
			return nil, err
		}
	}
	existing.Name = normalized.Name
	existing.Scene = normalized.Scene
	existing.TemplateCode = normalized.TemplateCode
	existing.TemplateParams = normalized.TemplateParams
	existing.Subject = normalized.Subject
	existing.Content = normalized.Content
	existing.IsDefault = normalized.IsDefault
	if err := s.templates.Update(ctx, existing); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, institutionID, "provider_template.update", existing.Name, map[string]interface{}{"provider_id": providerID, "scene": existing.Scene})
	return existing, nil
}

// Delete 删除模板；存在应用登录方式或 MFA 提供商引用时拒绝。
func (s *TemplateService) Delete(ctx context.Context, actor string, institutionID, providerID, templateID int64) error {
	existing, err := s.Get(ctx, institutionID, providerID, templateID)
	if err != nil {
		return err
	}
	if err := s.ensureNotReferenced(ctx, institutionID, existing.Channel, templateID); err != nil {
		return err
	}
	if err := s.templates.Delete(ctx, existing.Channel, templateID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "provider_template.delete", existing.Name, map[string]interface{}{"provider_id": providerID})
	return nil
}

// ResolveNotification 模板解析唯一入口：
//   - provider 必须同组织、启用、类别匹配；
//   - template 必须属于 provider 且 scene 匹配；
//   - templateID=0 时使用该 provider 在该 scene 的默认模板。
func (s *TemplateService) ResolveNotification(ctx context.Context, institutionID, providerID, templateID int64, scene string, expected providerpkg.Category) (*repository.Provider, *repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, nil, err
	}
	if providerpkg.NormalizeCategory(provider.Category) != string(expected) {
		return nil, nil, bizerrors.ErrInvalidParams.WithDetail("provider_category", provider.Category)
	}
	if !strings.EqualFold(provider.State, "active") {
		return nil, nil, bizerrors.ErrProviderUnavailable.WithDetail("provider", provider.Name)
	}
	channel := channelForCategory(provider.Category)
	var item *repository.ProviderTemplate
	if templateID > 0 {
		item, err = s.templates.GetByID(ctx, channel, templateID)
		if err != nil || item.InstitutionID != institutionID || item.ProviderID != providerID {
			return nil, nil, bizerrors.ErrInvalidParams.WithDetail("template_id", templateID)
		}
	} else {
		items, listErr := s.templates.ListByProvider(ctx, channel, institutionID, providerID, scene)
		if listErr != nil {
			return nil, nil, listErr
		}
		for _, candidate := range items {
			if candidate.IsDefault {
				item = candidate
				break
			}
		}
	}
	if item == nil {
		return nil, nil, bizerrors.ErrInvalidParams.WithDetail("template", "not_configured").WithDetail("scene", scene)
	}
	if item.Scene != scene {
		return nil, nil, bizerrors.ErrInvalidParams.WithDetail("template_scene", item.Scene).WithDetail("expected", scene)
	}
	for _, name := range item.TemplateParams {
		if !providerpkg.IsKnownTemplateVar(name) {
			return nil, nil, bizerrors.ErrInvalidParams.WithDetail("template_param", name)
		}
	}
	s.runtime.DecryptSecrets(provider)
	return provider, item, nil
}

func (s *TemplateService) provider(ctx context.Context, institutionID, providerID int64) (*repository.Provider, error) {
	item, err := s.providers.GetByID(ctx, providerID)
	if err != nil || item == nil || item.InstitutionID != institutionID {
		return nil, bizerrors.ErrNotFound.WithDetail("provider_id", providerID)
	}
	category := providerpkg.NormalizeCategory(item.Category)
	if category != string(providerpkg.CategoryEmail) && category != string(providerpkg.CategorySMS) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("category", item.Category)
	}
	return item, nil
}

func (s *TemplateService) clearDefault(ctx context.Context, channel string, institutionID, providerID int64, scene string, excludeID int64) error {
	items, err := s.templates.ListByProvider(ctx, channel, institutionID, providerID, scene)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == excludeID || !item.IsDefault {
			continue
		}
		item.IsDefault = false
		if err := s.templates.Update(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *TemplateService) ensureNotReferenced(ctx context.Context, institutionID int64, channel string, templateID int64) error {
	apps, err := s.apps.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 10000})
	if err != nil {
		return err
	}
	for _, app := range apps {
		for _, method := range app.SigninMethods {
			if method.TemplateID != templateID {
				continue
			}
			methodChannel := channelForMethod(method.Name)
			if methodChannel == channel {
				return bizerrors.New(bizerrors.CodeConflict, "template is referenced by an application sign-in method").
					WithDetail("application", app.Code).WithDetail("method", method.Name)
			}
		}
	}
	if channel != "sms" {
		return nil
	}
	providers, err := s.providers.List(ctx, repository.ListQuery{InstitutionID: institutionID, Field: "category", Value: string(providerpkg.CategoryMFA), Limit: 1000})
	if err != nil {
		return err
	}
	for _, item := range providers {
		if item.MfaSMSTemplateID == templateID {
			return bizerrors.New(bizerrors.CodeConflict, "template is referenced by an MFA provider").
				WithDetail("provider", item.Name)
		}
	}
	return nil
}

var templatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

func normalizeTemplateInput(category string, in NotificationTemplateInput) (NotificationTemplateInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Scene = strings.ToLower(strings.TrimSpace(in.Scene))
	in.TemplateCode = strings.TrimSpace(in.TemplateCode)
	in.Subject = strings.TrimSpace(in.Subject)
	if in.Name == "" {
		return in, bizerrors.ErrInvalidParams.WithDetail("required", "name")
	}
	if !repository.IsTemplateScene(in.Scene) {
		return in, bizerrors.ErrInvalidParams.WithDetail("scene", in.Scene)
	}
	seen := map[string]bool{}
	params := make([]string, 0, len(in.TemplateParams))
	for _, name := range in.TemplateParams {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if !providerpkg.IsKnownTemplateVar(name) {
			return in, bizerrors.ErrInvalidParams.WithDetail("template_param", name)
		}
		seen[name] = true
		params = append(params, name)
	}
	in.TemplateParams = params

	switch providerpkg.NormalizeCategory(category) {
	case string(providerpkg.CategorySMS):
		if in.TemplateCode == "" {
			return in, bizerrors.ErrInvalidParams.WithDetail("required", "template_code")
		}
		if len(in.TemplateParams) == 0 {
			in.TemplateParams = providerpkg.DefaultTemplateVars()
		}
	case string(providerpkg.CategoryEmail):
		if in.Content == "" {
			return in, bizerrors.ErrInvalidParams.WithDetail("required", "content")
		}
		for _, text := range []string{in.Subject, in.Content} {
			for _, match := range templatePlaceholderPattern.FindAllStringSubmatch(text, -1) {
				if !providerpkg.IsKnownTemplateVar(match[1]) {
					return in, bizerrors.ErrInvalidParams.WithDetail("template_param", match[1])
				}
			}
		}
	default:
		return in, bizerrors.ErrInvalidParams.WithDetail("category", category)
	}
	if in.Scene == repository.TemplateSceneMfaCode && providerpkg.NormalizeCategory(category) != string(providerpkg.CategorySMS) {
		return in, bizerrors.ErrInvalidParams.WithDetail("scene", in.Scene)
	}
	if in.Scene == repository.TemplateSceneMagicLink && providerpkg.NormalizeCategory(category) != string(providerpkg.CategoryEmail) {
		return in, bizerrors.ErrInvalidParams.WithDetail("scene", in.Scene)
	}
	return in, nil
}

var templateChannelByCategory = map[string]string{
	string(providerpkg.CategoryEmail): "email",
	string(providerpkg.CategorySMS):   "sms",
}

var templateChannelByMethod = map[string]string{
	repository.SigninMethodEmailCode: "email",
	repository.SigninMethodMagicLink: "email",
	repository.SigninMethodSMSCode:   "sms",
}

func channelForCategory(category string) string {
	return templateChannelByCategory[providerpkg.NormalizeCategory(category)]
}

func channelForMethod(name string) string {
	return templateChannelByMethod[name]
}
