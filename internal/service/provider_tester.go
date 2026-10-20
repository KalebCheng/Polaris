package service

import (
	"context"
	"strings"

	"polaris/internal/auth"
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
	category := providerpkg.NormalizeCategory(provider.Category)
	strategy, ok := providerTestStrategies[category]
	if !ok {
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported provider category").
			WithDetail("category", provider.Category)
	}
	checkErr := strategy(s, ctx, provider)
	if checkErr != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "provider check failed", checkErr).
			WithDetail("category", provider.Category).WithDetail("provider", provider.Name)
	}
	return nil
}

type providerTestStrategy func(*ProviderService, context.Context, *repository.Provider) error

var providerTestStrategies = map[string]providerTestStrategy{
	string(providerpkg.CategoryOAuth):   (*ProviderService).testSigninProvider,
	string(providerpkg.CategoryEmail):   (*ProviderService).testEmailProvider,
	string(providerpkg.CategorySMS):     (*ProviderService).testSMSProvider,
	string(providerpkg.CategoryCaptcha): (*ProviderService).testCaptchaProvider,
	string(providerpkg.CategoryStorage): (*ProviderService).testStorageProvider,
	string(providerpkg.CategoryMFA):     (*ProviderService).testMFAProvider,
}

func (s *ProviderService) testEmailProvider(ctx context.Context, provider *repository.Provider) error {
	tpl := s.defaultMessageTemplate(ctx, provider, repository.TemplateSceneLoginCode)
	return notify.Test(ctx, "email", notifyConfig(provider, tpl), provider.Receiver)
}

func (s *ProviderService) testSMSProvider(ctx context.Context, provider *repository.Provider) error {
	tpl := s.defaultMessageTemplate(ctx, provider, repository.TemplateSceneLoginCode)
	if tpl == nil {
		return bizerrors.ErrInvalidParams.WithDetail("template", "login_code").WithDetail("provider", provider.Name)
	}
	return notify.Test(ctx, "sms", notifyConfig(provider, tpl), provider.Receiver)
}

func (s *ProviderService) testCaptchaProvider(ctx context.Context, provider *repository.Provider) error {
	if s.captcha == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryCaptcha))
	}
	return s.captcha.Test(ctx, provider)
}

func (s *ProviderService) testStorageProvider(ctx context.Context, provider *repository.Provider) error {
	if s.storage == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryStorage))
	}
	return s.storage.Test(ctx, provider)
}

// notifyConfig 把提供商配置与消息模板映射为通知发送配置。
func notifyConfig(provider *repository.Provider, tpl *repository.ProviderTemplate) notify.Config {
	cfg := notify.Config{
		Name: provider.Name, Type: provider.Type,
		Host: provider.Host, Port: provider.Port, User: provider.ClientID, Password: provider.ClientSecret,
		SslMode: provider.SslMode, FromAddress: provider.FromAddress, RegionID: provider.RegionID,
		SignName: provider.SignName, AppID: provider.ClientID, AppSecret: provider.ClientSecret,
		Endpoint: provider.Endpoint,
	}
	if tpl != nil {
		cfg.Title = tpl.Subject
		cfg.Content = tpl.Content
		cfg.TemplateCode = tpl.TemplateCode
		cfg.TemplateParams = tpl.TemplateParams
	}
	return cfg
}

func (s *ProviderService) defaultMessageTemplate(ctx context.Context, provider *repository.Provider, scene string) *repository.ProviderTemplate {
	if s.templates == nil {
		return nil
	}
	channel := channelForCategory(provider.Category)
	items, err := s.templates.ListByProvider(ctx, channel, provider.InstitutionID, provider.ID, scene)
	if err != nil {
		return nil
	}
	for _, item := range items {
		if item.IsDefault {
			return item
		}
	}
	if len(items) > 0 {
		return items[0]
	}
	return nil
}

func (s *ProviderService) testMFAProvider(ctx context.Context, provider *repository.Provider) error {
	kind := providerpkg.NormalizeType(providerpkg.CategoryMFA, provider.Type)
	strategy, ok := mfaTestStrategies[kind]
	if !ok {
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported mfa provider type")
	}
	return strategy(s, ctx, provider)
}

var mfaTestStrategies = map[string]providerTestStrategy{
	providerpkg.TypeMFATOTP: (*ProviderService).testTOTPProvider,
	providerpkg.TypeMFASMS:  (*ProviderService).testSMSMFAProvider,
}

func (s *ProviderService) testTOTPProvider(_ context.Context, provider *repository.Provider) error {
	secret, _, err := auth.GenerateTOTPSecretWithOptions(provider.Name, "self-test", auth.TOTPOptions{
		Issuer: provider.MfaIssuer, Digits: provider.MfaDigits, Period: provider.MfaPeriod, Algorithm: provider.MfaAlgorithm,
	})
	if err != nil {
		return err
	}
	if secret == "" {
		return bizerrors.New(bizerrors.CodeProviderUnavailable, "empty totp secret")
	}
	return nil
}

func (s *ProviderService) testSMSMFAProvider(ctx context.Context, provider *repository.Provider) error {
	if s.templates == nil {
		return bizerrors.New(bizerrors.CodeProviderUnavailable, "template service unavailable")
	}
	smsProvider, err := s.providers.GetByID(ctx, provider.MfaSMSProviderID)
	if err != nil || smsProvider.InstitutionID != provider.InstitutionID ||
		providerpkg.NormalizeCategory(smsProvider.Category) != string(providerpkg.CategorySMS) {
		return bizerrors.ErrProviderUnavailable.WithDetail("mfa_sms_provider_id", provider.MfaSMSProviderID)
	}
	tpl, err := s.templates.GetByID(ctx, "sms", provider.MfaSMSTemplateID)
	if err != nil || tpl.ProviderID != smsProvider.ID || tpl.InstitutionID != provider.InstitutionID ||
		tpl.Scene != repository.TemplateSceneMfaCode {
		return bizerrors.ErrProviderUnavailable.WithDetail("mfa_sms_template_id", provider.MfaSMSTemplateID)
	}
	s.runtime.DecryptSecrets(smsProvider)
	cfg := notify.Config{
		Name: smsProvider.Name, Type: smsProvider.Type,
		Host: smsProvider.Host, Port: smsProvider.Port, User: smsProvider.ClientID, Password: smsProvider.ClientSecret,
		SslMode: smsProvider.SslMode, FromAddress: smsProvider.FromAddress, Title: tpl.Subject,
		Content: tpl.Content, RegionID: smsProvider.RegionID, SignName: smsProvider.SignName,
		TemplateCode: tpl.TemplateCode, AppID: smsProvider.ClientID, AppSecret: smsProvider.ClientSecret,
		Endpoint: smsProvider.Endpoint, TemplateParams: tpl.TemplateParams,
	}
	return notify.Test(ctx, "sms", cfg, provider.Receiver)
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
