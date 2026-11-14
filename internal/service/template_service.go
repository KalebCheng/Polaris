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
	signin    repository.InstitutionSigninMethodRepository
	runtime   *providerpkg.Runtime
	audits    *AuditRecorder
}

// NewTemplateService 创建通知模板服务。
func NewTemplateService(
	templates repository.ProviderTemplateRepository,
	providers repository.ProviderRepository,
	signin repository.InstitutionSigninMethodRepository,
	runtime *providerpkg.Runtime,
	audits *AuditRecorder,
) *TemplateService {
	return &TemplateService{templates: templates, providers: providers, signin: signin, runtime: runtime, audits: audits}
}

// NotificationTemplateInput 模板写入契约。
type NotificationTemplateInput struct {
	Name           string
	Scene          string
	TemplateCode   string
	TemplateParams []NotificationTemplateParam
	Subject        string
	Content        string
	IsDefault      bool
}

// NotificationTemplateParam 短信模板参数映射输入：提供商的变量名 → 平台占位符。
type NotificationTemplateParam struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

// Scenes 返回通知模板可用场景与场景内占位符（前端编辑器据此渲染，不在前端另抄一份）。
func (s *TemplateService) Scenes() []providerpkg.TemplateSceneInfo {
	return providerpkg.TemplateScenes()
}

// List 列出某提供商的模板（可按 scene 过滤）。
func (s *TemplateService) List(ctx context.Context, institutionID, providerID int64, scene string) ([]*repository.ProviderTemplate, error) {
	provider, err := s.provider(ctx, institutionID, providerID)
	if err != nil {
		return nil, err
	}
	channel := channelForCategory(provider.Category)
	if scene != "" && !providerpkg.TemplateSceneAllowed(channel, scene) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("scene", scene)
	}
	return s.templates.ListByProvider(ctx, channel, institutionID, providerID, scene)
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
		TemplateCode:   normalized.TemplateCode,
		TemplateParams: repositoryTemplateParams(normalized.TemplateParams),
		Subject:        normalized.Subject, Content: normalized.Content, IsDefault: normalized.IsDefault,
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
	existing.TemplateParams = repositoryTemplateParams(normalized.TemplateParams)
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
	switch channel {
	case "sms":
		seenSources := map[string]bool{}
		for _, param := range item.TemplateParams {
			if !providerpkg.TemplateVarAllowed(channel, scene, param.Source) {
				return nil, nil, bizerrors.ErrInvalidParams.WithDetail("template_param", param.Source)
			}
			seenSources[param.Source] = true
		}
		if required := providerpkg.RequiredTemplateVar(channel, scene); required != "" && !seenSources[required] {
			return nil, nil, bizerrors.ErrInvalidParams.WithDetail("required_template_param", required)
		}
	case "email":
		if err := validateEmailTemplateVars(item.Subject, item.Content, channel, scene); err != nil {
			return nil, nil, err
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
	methods, err := s.signin.List(ctx, institutionID)
	if err != nil {
		return err
	}
	for _, method := range methods {
		if channel == "email" && method.EmailTemplateID != nil && *method.EmailTemplateID == templateID {
			return bizerrors.New(bizerrors.CodeConflict, "template is referenced by an organization sign-in method").
				WithDetail("method_id", method.ID).WithDetail("method", method.Name)
		}
		if channel == "sms" && method.SMSTemplateID != nil && *method.SMSTemplateID == templateID {
			return bizerrors.New(bizerrors.CodeConflict, "template is referenced by an organization sign-in method").
				WithDetail("method_id", method.ID).WithDetail("method", method.Name)
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

var (
	templatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)
	templateParamKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

func normalizeTemplateInput(category string, in NotificationTemplateInput) (NotificationTemplateInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Scene = strings.ToLower(strings.TrimSpace(in.Scene))
	in.TemplateCode = strings.TrimSpace(in.TemplateCode)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Content = strings.TrimSpace(in.Content)
	if in.Name == "" {
		return in, bizerrors.ErrInvalidParams.WithDetail("required", "name")
	}
	channel := channelForCategory(category)
	if channel == "" || !providerpkg.TemplateSceneAllowed(channel, in.Scene) {
		return in, bizerrors.ErrInvalidParams.WithDetail("scene", in.Scene)
	}

	switch channel {
	case "sms":
		if in.TemplateCode == "" {
			return in, bizerrors.ErrInvalidParams.WithDetail("required", "template_code")
		}
		params, err := normalizeTemplateParams(in.TemplateParams, channel, in.Scene)
		if err != nil {
			return in, err
		}
		in.TemplateParams = params
		in.Subject = ""
		in.Content = ""
	case "email":
		if in.Content == "" {
			return in, bizerrors.ErrInvalidParams.WithDetail("required", "content")
		}
		content, err := sanitizeEmailHTML(in.Content)
		if err != nil {
			return in, err
		}
		in.Content = content
		in.TemplateParams = []NotificationTemplateParam{}
		if err := validateEmailTemplateVars(in.Subject, in.Content, channel, in.Scene); err != nil {
			return in, err
		}
	default:
		return in, bizerrors.ErrInvalidParams.WithDetail("category", category)
	}
	return in, nil
}

func normalizeTemplateParams(
	params []NotificationTemplateParam, channel, scene string,
) ([]NotificationTemplateParam, error) {
	requiredSource := providerpkg.RequiredTemplateVar(channel, scene)
	seenKeys := map[string]bool{}
	seenSources := map[string]bool{}
	out := make([]NotificationTemplateParam, 0, len(params))
	for _, item := range params {
		key := strings.TrimSpace(item.Key)
		source := strings.TrimSpace(item.Source)
		if key == "" && source == "" {
			continue
		}
		if key == "" {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "template_param.key")
		}
		if !templateParamKeyPattern.MatchString(key) {
			return nil, bizerrors.ErrInvalidParams.WithDetail("template_param.key", key)
		}
		if seenKeys[key] {
			return nil, bizerrors.ErrInvalidParams.WithDetail("duplicate_template_param", key)
		}
		if source == "" {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "template_param.source")
		}
		if !providerpkg.TemplateVarAllowed(channel, scene, source) {
			return nil, bizerrors.ErrInvalidParams.
				WithDetail("template_param", source).
				WithDetail("scene", scene)
		}
		seenKeys[key] = true
		seenSources[source] = true
		out = append(out, NotificationTemplateParam{Key: key, Source: source})
	}
	if len(out) == 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "template_params")
	}
	if requiredSource != "" && !seenSources[requiredSource] {
		return nil, bizerrors.ErrInvalidParams.
			WithDetail("required_template_param", requiredSource).
			WithDetail("scene", scene)
	}
	return out, nil
}

func validateEmailTemplateVars(subject, content, channel, scene string) error {
	requiredSource := providerpkg.RequiredTemplateVar(channel, scene)
	found := map[string]bool{}
	for _, text := range []string{subject, content} {
		for _, match := range templatePlaceholderPattern.FindAllStringSubmatch(text, -1) {
			key := strings.TrimSpace(match[1])
			if !providerpkg.TemplateVarAllowed(channel, scene, key) {
				return bizerrors.ErrInvalidParams.
					WithDetail("template_param", key).
					WithDetail("scene", scene)
			}
			found[key] = true
		}
	}
	if requiredSource != "" && !found[requiredSource] {
		return bizerrors.ErrInvalidParams.
			WithDetail("required_template_param", requiredSource).
			WithDetail("scene", scene)
	}
	return nil
}

func repositoryTemplateParams(params []NotificationTemplateParam) []repository.ProviderTemplateParam {
	out := make([]repository.ProviderTemplateParam, 0, len(params))
	for _, item := range params {
		out = append(out, repository.ProviderTemplateParam{Key: item.Key, Source: item.Source})
	}
	return out
}

func templateParamMap(params []repository.ProviderTemplateParam) map[string]string {
	out := make(map[string]string, len(params))
	for _, item := range params {
		out[item.Key] = item.Source
	}
	return out
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
