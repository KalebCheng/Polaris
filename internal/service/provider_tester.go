package service

import (
	"context"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/notify"
	providerpkg "polaris/internal/service/provider"
)

// Test 对提供商执行连通性/配置自检。类别不同，检测方式不同：
//   - Email：SMTP 连接 + 认证；
//   - SMS：阿里云/通用 HTTP 配置完整性（配置了 receiver 时真实发送）；
//   - Captcha：本地生成一次图片 / 远程校验站点密钥完整性；
//   - Storage：写入并删除探测对象；
//   - OAuth/OIDC：校验必需配置项。
func (s *ProviderService) Test(ctx context.Context, institutionID int64, name string) error {
	// 自检必须拿到**明文密钥**（浏览器/网关只看到 has_secret），因此经 Runtime 解密读取
	provider, err := s.runtime.Resolve(ctx, institutionID, name)
	if err != nil {
		return err
	}
	var checkErr error
	switch providerpkg.NormalizeCategory(provider.Category) {
	case string(providerpkg.CategoryEmail):
		checkErr = notify.Test(ctx, "email", notifyConfig(provider), provider.Receiver)
	case string(providerpkg.CategorySMS):
		checkErr = notify.Test(ctx, "sms", notifyConfig(provider), provider.Receiver)
	case string(providerpkg.CategoryCaptcha):
		if s.captcha == nil {
			return bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryCaptcha))
		}
		checkErr = s.captcha.Test(ctx, provider)
	case string(providerpkg.CategoryStorage):
		if s.storage == nil {
			return bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryStorage))
		}
		checkErr = s.storage.Test(ctx, provider)
	case string(providerpkg.CategoryOAuth):
		checkErr = s.testSigninProvider(ctx, provider)
	default:
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported provider category").
			WithDetail("category", provider.Category)
	}
	if checkErr != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "provider check failed", checkErr).
			WithDetail("category", provider.Category).WithDetail("provider", provider.Name)
	}
	return nil
}

// notifyConfig 把提供商配置映射为通知发送配置
func notifyConfig(provider *repository.Provider) notify.Config {
	return notify.Config{
		Name: provider.Name, Type: provider.Type,
		Host: provider.Host, Port: provider.Port, User: provider.ClientID, Password: provider.ClientSecret,
		SslMode: provider.SslMode, FromAddress: provider.FromAddress, Title: provider.Title,
		Content: provider.Content, RegionID: provider.RegionID, SignName: provider.SignName,
		TemplateCode: provider.TemplateCode, AppID: provider.ClientID, AppSecret: provider.ClientSecret,
		Endpoint: provider.Endpoint,
	}
}

// testSigninProvider 校验外部身份提供方：配置完整性 + 能否真实构造连接器
// （OIDC 会真的执行一次 discovery；任何失败都如实返回，不伪装成功）。
func (s *ProviderService) testSigninProvider(ctx context.Context, provider *repository.Provider) error {
	if strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.ClientSecret) == "" {
		return bizerrors.New(bizerrors.CodeInvalidParams, "client id and secret are required")
	}
	kind := providerpkg.Kind(provider)
	if kind == providerpkg.TypeOIDC && strings.TrimSpace(provider.IssuerURL) == "" &&
		(strings.TrimSpace(provider.CustomAuthURL) == "" || strings.TrimSpace(provider.CustomTokenURL) == "") {
		return bizerrors.New(bizerrors.CodeInvalidParams, "issuer url or custom endpoints are required for oidc provider")
	}
	if s.resolver == nil {
		return nil
	}
	if _, err := s.resolver.Get(ctx, provider); err != nil {
		return err
	}
	return nil
}
