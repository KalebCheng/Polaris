package service

import (
	"context"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// ProviderService 提供商服务：覆盖外部身份提供方（第三方登录 / 标准 OIDC）、
// 邮件、短信、人机校验、存储五类。
//
// 字段契约由 internal/service/provider 的目录（catalog）声明：请求体为
// config（非敏感）+ secrets（敏感明文）+ policy（SSO 信任策略），落库列由目录的 store 决定。
// 因此新增一个提供商类型只需改目录声明，服务层与前端都不必动。
type ProviderService struct {
	providers repository.ProviderRepository
	orgs      repository.InstitutionRepository
	runtime   *providerpkg.Runtime
	resolver  *providerpkg.Resolver
	captcha   *CaptchaService
	storage   *StorageService
	audits    *AuditRecorder
}

// NewProviderService 创建提供商服务
func NewProviderService(
	providers repository.ProviderRepository,
	orgs repository.InstitutionRepository,
	runtime *providerpkg.Runtime,
	resolver *providerpkg.Resolver,
	captcha *CaptchaService,
	storage *StorageService,
	audits *AuditRecorder,
) *ProviderService {
	return &ProviderService{
		providers: providers, orgs: orgs, runtime: runtime, resolver: resolver,
		captcha: captcha, storage: storage, audits: audits,
	}
}

// ProviderInput 提供商的对外输入契约（目录 schema 驱动）
type ProviderInput struct {
	InstitutionID int64
	Name          string
	DisplayName   string
	Category      string
	Type          string
	State         string
	// Config 非敏感配置（键由类型目录声明）
	Config map[string]any
	// Secrets 敏感配置（明文；留空 = 保持原值）
	Secrets map[string]string
	// Policy SSO 信任策略（仅外部身份提供方；键由类型目录声明）
	Policy map[string]any
}

// Catalog 返回提供商目录（类别 / 类型 / 字段 schema；前端唯一来源）
func (s *ProviderService) Catalog() providerpkg.Catalog { return providerpkg.Build() }

// resolveType 归一化并校验类别与类型（未知取值一律拒绝，避免"能选但没实现"）
func resolveType(category, typeKey string) (providerpkg.Category, string, providerpkg.TypeInfo, error) {
	normalized := providerpkg.NormalizeCategory(category)
	if normalized == "" {
		return "", "", providerpkg.TypeInfo{}, bizerrors.ErrInvalidParams.WithDetail("category", category)
	}
	key := providerpkg.NormalizeType(providerpkg.Category(normalized), typeKey)
	if key == "" {
		return "", "", providerpkg.TypeInfo{}, bizerrors.ErrInvalidParams.WithDetail("type", typeKey)
	}
	info, ok := providerpkg.Find(providerpkg.Category(normalized), key)
	if !ok {
		return "", "", providerpkg.TypeInfo{}, bizerrors.ErrInvalidParams.WithDetail("type", typeKey)
	}
	return providerpkg.Category(normalized), key, info, nil
}

// Create 创建提供商
func (s *ProviderService) Create(ctx context.Context, actor string, in ProviderInput) (*repository.Provider, error) {
	category, typeKey, info, err := resolveType(in.Category, in.Type)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "name")
	}
	if in.InstitutionID == 0 {
		return nil, bizerrors.ErrInvalidParams.WithDetail("required", "institution_id")
	}
	if _, err := s.orgs.GetByID(ctx, in.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", in.InstitutionID)
	}
	if _, err := s.providers.GetByName(ctx, in.InstitutionID, name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "provider already exists")
	}
	item := &repository.Provider{
		InstitutionID: in.InstitutionID,
		Name:          name,
		DisplayName:   strings.TrimSpace(in.DisplayName),
		Category:      string(category),
		Type:          typeKey,
		State:         normalizeProviderState(in.State),
	}
	if err := providerpkg.Bind(item, info, in.Config, in.Policy, in.Secrets, providerpkg.BindOptions{}); err != nil {
		return nil, err
	}
	// 单例类别（邮件/短信/人机校验/存储）同一组织最多一个启用项，否则"到底用哪个"就靠排序碰运气
	if err := s.ensureSingletonActive(ctx, item.InstitutionID, category, item.State, 0); err != nil {
		return nil, err
	}
	if err := s.runtime.EncryptSecrets(item); err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "failed to protect provider secret", err)
	}
	if _, err := s.providers.Create(ctx, item); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, item.InstitutionID, "provider.create", item.Name, nil)
	return s.present(item), nil
}

// Update 更新提供商（组织与标识不可变；secrets 留空表示保持原值）
func (s *ProviderService) Update(ctx context.Context, actor string, institutionID int64, name string, in ProviderInput) (*repository.Provider, error) {
	existing, err := s.providers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	// 类别与类型不允许在编辑时漂移（否则旧列里的配置会残留成"看不懂的脏数据"）
	category, typeKey, info, err := resolveType(existing.Category, existing.Type)
	if err != nil {
		return nil, err
	}
	if in.Category != "" && providerpkg.NormalizeCategory(in.Category) != string(category) {
		return nil, bizerrors.ErrInvalidParams.WithDetail("category", in.Category)
	}
	if in.Type != "" && providerpkg.NormalizeType(category, in.Type) != typeKey {
		return nil, bizerrors.ErrInvalidParams.WithDetail("type", in.Type)
	}
	if strings.TrimSpace(in.DisplayName) != "" || in.DisplayName == "" {
		existing.DisplayName = strings.TrimSpace(in.DisplayName)
	}
	if strings.TrimSpace(in.State) != "" {
		existing.State = normalizeProviderState(in.State)
	}
	// 实体上的密文不能参与"必填校验"的判断，先标记密钥是否已配置
	existing.HasSecret = strings.TrimSpace(existing.ClientSecret) != ""
	if err := providerpkg.Bind(existing, info, in.Config, in.Policy, in.Secrets,
		providerpkg.BindOptions{KeepSecretsOnEmpty: true}); err != nil {
		return nil, err
	}
	if err := s.ensureSingletonActive(ctx, institutionID, category, existing.State, existing.ID); err != nil {
		return nil, err
	}
	if err := s.runtime.EncryptSecrets(existing); err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "failed to protect provider secret", err)
	}
	if err := s.providers.Update(ctx, existing); err != nil {
		return nil, err
	}
	// 本轮变更立即生效（本实例）；跨实例由连接器缓存的 TTL 兜底
	if s.resolver != nil {
		s.resolver.Invalidate(institutionID, existing.Name)
	}
	s.audits.Record(ctx, actor, institutionID, "provider.update", existing.Name, nil)
	return s.present(existing), nil
}

// Get 查询提供商（密钥已剥离，仅以 has_secret 表达是否已配置）
func (s *ProviderService) Get(ctx context.Context, institutionID int64, name string) (*repository.Provider, error) {
	item, err := s.providers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	return s.present(item), nil
}

// List 分页列出提供商（category 非空时按类别过滤；取值按目录归一化）
func (s *ProviderService) List(ctx context.Context, institutionID int64, keyword, category string, offset, limit int) ([]*repository.Provider, int64, error) {
	query := institutionQuery(institutionID, keyword, offset, limit)
	if normalized := providerpkg.NormalizeCategory(category); normalized != "" {
		query.Field = "category"
		query.Value = normalized
	}
	items, err := s.providers.List(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.providers.Count(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		items[i] = s.present(items[i])
	}
	return items, total, nil
}

// ListByCategory 按类别列出启用的提供商（应用向导等按需过滤用）
func (s *ProviderService) ListByCategory(ctx context.Context, institutionID int64, category string) ([]*repository.Provider, error) {
	items, err := s.providers.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "category", Value: providerpkg.NormalizeCategory(category), Limit: 200,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*repository.Provider, 0, len(items))
	for _, item := range items {
		if !strings.EqualFold(item.State, "active") {
			continue
		}
		out = append(out, s.present(item))
	}
	return out, nil
}

// SetState 只切换启用状态（列表卡片上的快捷开关）。
//
// 为什么不复用 Update：Update 的语义是"整表提交"（display_name 等顶层字段按提交值覆盖），
// 只带一个 state 去 PUT 会把名称清空。切换状态是运维高频动作，单独一个入口更不容易出错。
func (s *ProviderService) SetState(ctx context.Context, actor string, institutionID int64, name, state string) (*repository.Provider, error) {
	item, err := s.providers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	next := normalizeProviderState(state)
	category := providerpkg.Category(providerpkg.NormalizeCategory(item.Category))
	if err := s.ensureSingletonActive(ctx, institutionID, category, next, item.ID); err != nil {
		return nil, err
	}
	item.State = next
	if err := s.providers.Update(ctx, item); err != nil {
		return nil, err
	}
	if s.resolver != nil {
		s.resolver.Invalidate(institutionID, item.Name)
	}
	s.audits.Record(ctx, actor, institutionID, "provider.set_state", item.Name, map[string]interface{}{"state": next})
	return s.present(item), nil
}

// ensureSingletonActive 单例类别在同一组织内只允许一个启用项。
// excludeID 用于更新/切换状态时排除自身。
func (s *ProviderService) ensureSingletonActive(ctx context.Context, institutionID int64, category providerpkg.Category, state string, excludeID int64) error {
	if !providerpkg.IsSingleton(category) || !strings.EqualFold(state, "active") {
		return nil
	}
	items, err := s.providers.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "category", Value: string(category), Limit: 200,
	})
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == excludeID || !strings.EqualFold(item.State, "active") {
			continue
		}
		return bizerrors.New(bizerrors.CodeConflict,
			"another provider of this category is already active").
			WithDetail("category", string(category)).
			WithDetail("active_provider", item.Name)
	}
	return nil
}

// Delete 删除提供商
func (s *ProviderService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	item, err := s.providers.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	if err := s.providers.Delete(ctx, item.ID); err != nil {
		return err
	}
	if s.resolver != nil {
		s.resolver.Invalidate(institutionID, name)
	}
	s.audits.Record(ctx, actor, institutionID, "provider.delete", name, nil)
	return nil
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *ProviderService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.providers.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}

// present 剥离密钥（响应体不得出现明文或密文，只以 has_secret 表达）
func (s *ProviderService) present(item *repository.Provider) *repository.Provider {
	providerpkg.StripSecrets(item)
	return item
}

func normalizeProviderState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "disabled", "inactive":
		return "disabled"
	default:
		return "active"
	}
}
