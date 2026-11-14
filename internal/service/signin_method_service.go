package service

import (
	"context"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// SigninMethodInput 组织级登录方式写入契约。
type SigninMethodInput struct {
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	Identifiers     []string `json:"identifiers"`
	ProviderID      *int64   `json:"provider_id"`
	EmailTemplateID *int64   `json:"email_template_id"`
	SMSTemplateID   *int64   `json:"sms_template_id"`
	TTLSeconds      int      `json:"ttl_seconds"`
	MfaEnabled      bool     `json:"mfa_enabled"`
	MfaRequired     bool     `json:"mfa_required"`
	MfaProviderIDs  []int64  `json:"mfa_provider_ids"`
	SortOrder       int      `json:"sort_order"`
}

// SigninMethodService 组织级登录方式的 CRUD、校验与默认值。
type SigninMethodService struct {
	methods   repository.InstitutionSigninMethodRepository
	providers repository.ProviderRepository
	templates *TemplateService
	audits    *AuditRecorder
}

// NewSigninMethodService 创建组织登录方式服务。
func NewSigninMethodService(
	methods repository.InstitutionSigninMethodRepository,
	providers repository.ProviderRepository,
	templates *TemplateService,
	audits *AuditRecorder,
) *SigninMethodService {
	return &SigninMethodService{methods: methods, providers: providers, templates: templates, audits: audits}
}

// List 列出组织全部登录方式。
func (s *SigninMethodService) List(ctx context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	return s.methods.List(ctx, institutionID)
}

// Options 列出应用向导可选的启用方法。
func (s *SigninMethodService) Options(ctx context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	items, err := s.methods.ListActive(ctx, institutionID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// 应用向导只需要摘要；MFA provider 明细不在 options 暴露。
		item.MfaProviderIDs = nil
	}
	return items, nil
}

// SelectedByApplications 批量返回应用选中的启用方法（用于应用列表摘要）。
func (s *SigninMethodService) SelectedByApplications(ctx context.Context, institutionID int64, applicationIDs []int64) (map[int64][]repository.SigninMethod, error) {
	raw, err := s.methods.ListByApplications(ctx, institutionID, applicationIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]repository.SigninMethod, len(raw))
	for appID, items := range raw {
		for _, item := range items {
			if item.IsActive() {
				out[appID] = append(out[appID], repository.EffectiveSigninMethod(item))
			}
		}
	}
	return out, nil
}

// Create 创建组织登录方式。
func (s *SigninMethodService) Create(ctx context.Context, actor string, institutionID int64, in SigninMethodInput) (*repository.InstitutionSigninMethod, error) {
	method, err := s.build(ctx, institutionID, nil, in)
	if err != nil {
		return nil, err
	}
	if err := s.ensureUnique(ctx, method, 0); err != nil {
		return nil, err
	}
	id, err := s.methods.Create(ctx, method)
	if err != nil {
		return nil, err
	}
	method.ID = id
	s.audits.Record(ctx, actor, institutionID, "institution.signin_method.create", method.Name, map[string]interface{}{
		"method_id": id, "provider_id": method.ProviderID,
	})
	return s.methods.GetByID(ctx, institutionID, id)
}

// Update 更新组织登录方式；name 与 Provider 的 provider_id 不可变。
func (s *SigninMethodService) Update(ctx context.Context, actor string, institutionID, id int64, in SigninMethodInput) (*repository.InstitutionSigninMethod, error) {
	existing, err := s.methods.GetByID(ctx, institutionID, id)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("method_id", id)
	}
	if strings.TrimSpace(in.Name) != "" && in.Name != existing.Name {
		return nil, bizerrors.ErrInvalidParams.WithDetail("name", in.Name)
	}
	if existing.Name == repository.SigninMethodProvider {
		if in.ProviderID == nil || existing.ProviderID == nil || *in.ProviderID != *existing.ProviderID {
			return nil, bizerrors.ErrInvalidParams.WithDetail("provider_id", "immutable for provider methods")
		}
	}
	method, err := s.build(ctx, institutionID, existing, in)
	if err != nil {
		return nil, err
	}
	if err := s.ensureUnique(ctx, method, id); err != nil {
		return nil, err
	}
	if err := s.methods.Update(ctx, method); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, institutionID, "institution.signin_method.update", method.Name, map[string]interface{}{
		"method_id": id, "provider_id": method.ProviderID,
	})
	return s.methods.GetByID(ctx, institutionID, id)
}

// SetState 启停组织登录方式。
func (s *SigninMethodService) SetState(ctx context.Context, actor string, institutionID, id int64, state string) error {
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "active" && state != "disabled" {
		return bizerrors.ErrInvalidParams.WithDetail("state", state)
	}
	existing, err := s.methods.GetByID(ctx, institutionID, id)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("method_id", id)
	}
	if state == "disabled" && existing.IsActive() {
		if err := s.ensureNotLastActive(ctx, institutionID, id); err != nil {
			return err
		}
	}
	if err := s.methods.SetState(ctx, institutionID, id, state); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "institution.signin_method.state", existing.Name, map[string]interface{}{
		"method_id": id, "state": state,
	})
	return nil
}

// Delete 删除未被应用引用的组织登录方式。
func (s *SigninMethodService) Delete(ctx context.Context, actor string, institutionID, id int64) error {
	existing, err := s.methods.GetByID(ctx, institutionID, id)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("method_id", id)
	}
	apps, err := s.methods.ListApplicationsByMethod(ctx, institutionID, id)
	if err != nil {
		return err
	}
	if len(apps) > 0 {
		codes := make([]string, 0, len(apps))
		for _, app := range apps {
			codes = append(codes, app.Code)
		}
		return bizerrors.New(bizerrors.CodeConflict, "sign-in method is referenced by applications").
			WithDetail("method_id", id).WithDetail("applications", codes)
	}
	if existing.IsActive() {
		if err := s.ensureNotLastActive(ctx, institutionID, id); err != nil {
			return err
		}
	}
	if err := s.methods.Delete(ctx, institutionID, id); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "institution.signin_method.delete", existing.Name, map[string]interface{}{
		"method_id": id, "provider_id": existing.ProviderID,
	})
	return nil
}

// ValidateSelection 校验应用选择：同组织、启用、至少一条、去重。
func (s *SigninMethodService) ValidateSelection(ctx context.Context, institutionID int64, ids []int64) ([]*repository.InstitutionSigninMethod, error) {
	ids = dedupeInt64(ids)
	if len(ids) == 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "signin_method_ids")
	}
	items, err := s.methods.ListByIDs(ctx, institutionID, ids)
	if err != nil {
		return nil, err
	}
	if len(items) != len(ids) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("signin_method_ids", "contains unknown method")
	}
	for _, item := range items {
		if !item.IsActive() {
			return nil, bizerrors.ErrInvalidParams.WithDetail("state", "selected method is disabled").
				WithDetail("method_id", item.ID)
		}
	}
	return items, nil
}

// DefaultMethodIDs 返回组织当前全部启用方法（新应用默认全选）。
func (s *SigninMethodService) DefaultMethodIDs(ctx context.Context, institutionID int64) ([]int64, error) {
	items, err := s.methods.ListActive(ctx, institutionID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

// EnsureDefaultPassword 确保组织至少有一条默认 Password 方法（新建组织/seed 用）。
func (s *SigninMethodService) EnsureDefaultPassword(ctx context.Context, institutionID int64) error {
	items, err := s.methods.List(ctx, institutionID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Name == repository.SigninMethodPassword {
			return nil
		}
	}
	_, err = s.Create(ctx, "system", institutionID, SigninMethodInput{
		Name:        repository.SigninMethodPassword,
		DisplayName: "Password",
		Identifiers: []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail},
		MfaEnabled:  false,
		SortOrder:   10,
	})
	return err
}

func (s *SigninMethodService) build(ctx context.Context, institutionID int64, existing *repository.InstitutionSigninMethod, in SigninMethodInput) (*repository.InstitutionSigninMethod, error) {
	name := in.Name
	if existing != nil {
		name = existing.Name
	}
	name = strings.TrimSpace(name)
	if !implementedSigninMethods[name] {
		return nil, bizerrors.ErrInvalidParams.WithDetail("name", name)
	}
	method := &repository.InstitutionSigninMethod{
		ID:            idOfMethod(existing),
		InstitutionID: institutionID,
		Name:          name,
		DisplayName:   strings.TrimSpace(in.DisplayName),
		TTLSeconds:    in.TTLSeconds,
		MfaEnabled:    in.MfaEnabled,
		MfaRequired:   in.MfaRequired,
		SortOrder:     in.SortOrder,
		State:         "active",
	}
	if existing != nil {
		method.State = existing.State
		method.SortOrder = firstNonZero(in.SortOrder, existing.SortOrder)
	}
	if method.SortOrder <= 0 {
		method.SortOrder = s.nextSortOrder(ctx, institutionID)
	}
	if method.MfaRequired && !method.MfaEnabled {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "mfa_enabled")
	}

	switch name {
	case repository.SigninMethodPassword:
		ids, err := normalizePasswordIdentifiers(in.Identifiers)
		if err != nil {
			return nil, err
		}
		method.Identifiers = ids
		method.ProviderID = nil
		method.EmailTemplateID = nil
		method.SMSTemplateID = nil
		method.TTLSeconds = 0
		if method.DisplayName == "" {
			method.DisplayName = "Password"
		}
	case repository.SigninMethodEmailCode, repository.SigninMethodMagicLink:
		if in.ProviderID == nil || *in.ProviderID <= 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "provider_id")
		}
		if in.EmailTemplateID == nil || *in.EmailTemplateID <= 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "email_template_id")
		}
		scene := repository.TemplateSceneLoginCode
		if name == repository.SigninMethodMagicLink {
			scene = repository.TemplateSceneMagicLink
		}
		provider, tpl, err := s.templates.ResolveNotification(ctx, institutionID, *in.ProviderID, *in.EmailTemplateID,
			scene, providerpkg.CategoryEmail)
		if err != nil {
			return nil, err
		}
		if provider == nil || tpl == nil {
			return nil, bizerrors.ErrInvalidParams.WithDetail("email_template_id", *in.EmailTemplateID)
		}
		method.ProviderID = in.ProviderID
		method.EmailTemplateID = in.EmailTemplateID
		method.SMSTemplateID = nil
		method.TTLSeconds = clampTTL(in.TTLSeconds, 30, 3600, 300)
		if method.DisplayName == "" {
			method.DisplayName = firstNonEmptyStr(tpl.Name, defaultMethodDisplayName(method, nil))
		}
	case repository.SigninMethodSMSCode:
		if in.ProviderID == nil || *in.ProviderID <= 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "provider_id")
		}
		if in.SMSTemplateID == nil || *in.SMSTemplateID <= 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "sms_template_id")
		}
		provider, tpl, err := s.templates.ResolveNotification(ctx, institutionID, *in.ProviderID, *in.SMSTemplateID,
			repository.TemplateSceneLoginCode, providerpkg.CategorySMS)
		if err != nil {
			return nil, err
		}
		if provider == nil || tpl == nil {
			return nil, bizerrors.ErrInvalidParams.WithDetail("sms_template_id", *in.SMSTemplateID)
		}
		method.ProviderID = in.ProviderID
		method.EmailTemplateID = nil
		method.SMSTemplateID = in.SMSTemplateID
		method.TTLSeconds = clampTTL(in.TTLSeconds, 30, 1800, 300)
		if method.DisplayName == "" {
			method.DisplayName = firstNonEmptyStr(tpl.Name, defaultMethodDisplayName(method, nil))
		}
	case repository.SigninMethodProvider:
		if in.ProviderID == nil || *in.ProviderID <= 0 {
			return nil, bizerrors.ErrInvalidParams.WithDetail("required", "provider_id")
		}
		provider, err := s.providers.GetByID(ctx, *in.ProviderID)
		if err != nil || provider == nil || provider.InstitutionID != institutionID ||
			!strings.EqualFold(provider.State, "active") ||
			providerpkg.NormalizeCategory(provider.Category) != string(providerpkg.CategoryOAuth) {
			return nil, bizerrors.ErrInvalidParams.WithDetail("provider_id", *in.ProviderID)
		}
		method.ProviderID = in.ProviderID
		method.EmailTemplateID = nil
		method.SMSTemplateID = nil
		method.TTLSeconds = 0
		method.Identifiers = nil
		if method.DisplayName == "" {
			method.DisplayName = firstNonEmptyStr(provider.DisplayName, provider.Name)
		}
	}

	if method.MfaEnabled {
		ids, err := s.normalizeMFAProviders(ctx, institutionID, in.MfaProviderIDs)
		if err != nil {
			return nil, err
		}
		method.MfaProviderIDs = ids
	} else {
		method.MfaRequired = false
		method.MfaProviderIDs = []int64{}
	}
	return method, nil
}

func (s *SigninMethodService) ensureUnique(ctx context.Context, method *repository.InstitutionSigninMethod, excludeID int64) error {
	items, err := s.methods.List(ctx, method.InstitutionID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == excludeID {
			continue
		}
		if item.Name == method.Name {
			if method.Name != repository.SigninMethodProvider {
				return bizerrors.New(bizerrors.CodeConflict, "sign-in method already exists").
					WithDetail("name", method.Name)
			}
			if item.ProviderID != nil && method.ProviderID != nil && *item.ProviderID == *method.ProviderID {
				return bizerrors.New(bizerrors.CodeConflict, "provider sign-in method already exists").
					WithDetail("provider_id", *method.ProviderID)
			}
		}
	}
	return nil
}

func (s *SigninMethodService) ensureNotLastActive(ctx context.Context, institutionID, excludeID int64) error {
	items, err := s.methods.ListActive(ctx, institutionID)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range items {
		if item.ID != excludeID {
			count++
		}
	}
	if count == 0 {
		return bizerrors.New(bizerrors.CodeConflict, "at least one active sign-in method is required")
	}
	return nil
}

func (s *SigninMethodService) normalizeMFAProviders(ctx context.Context, institutionID int64, ids []int64) ([]int64, error) {
	ids = dedupeInt64(ids)
	if len(ids) == 0 {
		ids = s.defaultMFAProviderIDs(ctx, institutionID)
	}
	if len(ids) == 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "mfa_provider_ids")
	}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		provider, err := s.providers.GetByID(ctx, id)
		if err != nil || provider == nil || provider.InstitutionID != institutionID ||
			!strings.EqualFold(provider.State, "active") ||
			providerpkg.NormalizeCategory(provider.Category) != string(providerpkg.CategoryMFA) {
			return nil, bizerrors.ErrInvalidParams.WithDetail("mfa_provider_id", id)
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *SigninMethodService) defaultMFAProviderIDs(ctx context.Context, institutionID int64) []int64 {
	if s.providers == nil {
		return []int64{}
	}
	items, err := s.providers.List(ctx, repository.ListQuery{
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

func (s *SigninMethodService) nextSortOrder(ctx context.Context, institutionID int64) int {
	items, err := s.methods.List(ctx, institutionID)
	if err != nil {
		return 10
	}
	max := 0
	for _, item := range items {
		if item.SortOrder > max {
			max = item.SortOrder
		}
	}
	return max + 10
}

func normalizePasswordIdentifiers(items []string) ([]string, error) {
	values := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if !repository.IsPasswordIdentifier(item) {
			return nil, bizerrors.ErrInvalidParams.WithDetail("identifier", item)
		}
		if !seen[item] {
			seen[item] = true
			values = append(values, item)
		}
	}
	if len(values) == 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "identifiers")
	}
	ordered := make([]string, 0, len(values))
	for _, item := range []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail, repository.PasswordIdentifierPhone} {
		if seen[item] {
			ordered = append(ordered, item)
		}
	}
	return ordered, nil
}

func clampTTL(value, min, max, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func idOfMethod(method *repository.InstitutionSigninMethod) int64 {
	if method == nil {
		return 0
	}
	return method.ID
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
