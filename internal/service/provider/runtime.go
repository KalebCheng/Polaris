package provider

import (
	"context"
	"strings"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// Runtime 是提供商的**统一运行时读取入口**：按组织 + 类别/名称取行，并解密敏感字段。
//
// 为什么要有它（历史教训）：通知、存储、人机校验、外部身份提供方原先各自拼
// `providers.List(ListQuery{Field:"category", Value:"Email"})` 之类的查询，类别字面量大小写不一、
// 关键在"谁在读、读的是什么"只能靠 grep。集中一处后：类别常量单一来源、密钥解密只有一处、
// 新增读取点不必再抄一遍查询。
type Runtime struct {
	providers repository.ProviderRepository
	cipher    *auth.SecretCipher
}

// NewRuntime 创建运行时读取器（cipher 可为 nil，此时不做加解密，仅用于测试）
func NewRuntime(providers repository.ProviderRepository, cipher *auth.SecretCipher) *Runtime {
	return &Runtime{providers: providers, cipher: cipher}
}

// Active 返回组织内某类别首个启用的提供商（密钥已解密）；不存在返回 ErrProviderUnavailable。
func (r *Runtime) Active(ctx context.Context, institutionID int64, category Category) (*repository.Provider, error) {
	items, err := r.ListActive(ctx, institutionID, category)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("category", string(category))
	}
	return items[0], nil
}

// ListActive 列出某类别全部启用的提供商（密钥已解密，按类别+名称排序）。
//
// institutionID == 0 表示**跨组织**：登录页未指定组织时列出全部，或在"只带了提供商名"时按名称定位
// （`AuthService.resolveSigninInstitutionID`）。**不能把 0 当成错误**——第四阶段这里写成了硬拒绝，
// 导致"不带 institution 发起 SSO"必然 404（跨组织解析成了死代码）。
func (r *Runtime) ListActive(ctx context.Context, institutionID int64, category Category) ([]*repository.Provider, error) {
	if r.providers == nil {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("category", string(category))
	}
	query := repository.ListQuery{
		Field: "category",
		Value: string(category),
		Limit: 200,
	}
	if institutionID != 0 {
		query.InstitutionID = institutionID
	}
	items, err := r.providers.List(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]*repository.Provider, 0, len(items))
	for _, item := range items {
		if !strings.EqualFold(item.State, "active") {
			continue
		}
		r.DecryptSecrets(item)
		out = append(out, item)
	}
	return out, nil
}

// Resolve 按 (组织, 名称) 取提供商（密钥已解密）。
func (r *Runtime) Resolve(ctx context.Context, institutionID int64, name string) (*repository.Provider, error) {
	if r.providers == nil || institutionID == 0 || name == "" {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	item, err := r.providers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	r.DecryptSecrets(item)
	return item, nil
}

// FindSignin 在组织内按名称解析可用于登录的外部身份提供方（类别 oauth 且启用）。
func (r *Runtime) FindSignin(ctx context.Context, institutionID int64, name string) (*repository.Provider, error) {
	item, err := r.Resolve(ctx, institutionID, name)
	if err != nil {
		return nil, err
	}
	if NormalizeCategory(item.Category) != string(CategoryOAuth) {
		return nil, bizerrors.ErrNotFound.WithDetail("provider", name)
	}
	if !strings.EqualFold(item.State, "active") {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("provider", name)
	}
	return item, nil
}

// DecryptSecrets 就地把密文密钥解密为明文（仅内存；出服务层前必须由 StripSecrets 剥离）。
//
// 过渡期兼容：迁移 027 只改结构，存量行的明文密钥尚未回填为密文，
// 解密失败时保持原样（视为明文），由 `-rotate-provider-secrets` 子命令完成回填。
func (r *Runtime) DecryptSecrets(item *repository.Provider) {
	if item == nil || item.ClientSecret == "" {
		return
	}
	if r.cipher == nil {
		item.HasSecret = true
		return
	}
	if plain, err := r.cipher.Decrypt(item.ClientSecret); err == nil {
		item.ClientSecret = plain
	}
	item.HasSecret = true
}

// EncryptSecrets 加密实体上的明文密钥（落库前调用；已加密或为空则原样返回）。
func (r *Runtime) EncryptSecrets(item *repository.Provider) error {
	if item == nil || item.ClientSecret == "" || r.cipher == nil {
		return nil
	}
	if _, err := r.cipher.Decrypt(item.ClientSecret); err == nil {
		return nil // 已是密文
	}
	encrypted, err := r.cipher.Encrypt(item.ClientSecret)
	if err != nil {
		return err
	}
	item.ClientSecret = encrypted
	return nil
}

// StripSecrets 剥离密钥并把"是否已配置"记录到 HasSecret（响应 DTO 唯一可用的表达）。
func StripSecrets(item *repository.Provider) {
	if item == nil {
		return
	}
	if item.ClientSecret != "" {
		item.HasSecret = true
	}
	item.ClientSecret = ""
}

// Kind 归一化某行提供商的类型 key（type 优先，历史 sub_type 已由迁移 027 合并）。
func Kind(item *repository.Provider) string {
	if item == nil {
		return ""
	}
	if category := NormalizeCategory(item.Category); category != "" {
		if kind := NormalizeType(Category(category), item.Type); kind != "" {
			return kind
		}
		return strings.ToLower(strings.TrimSpace(item.Type))
	}
	return strings.ToLower(strings.TrimSpace(item.Type))
}
