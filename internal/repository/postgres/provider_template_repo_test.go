package postgres

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

// 回归：短信参数映射是 JSONB 数组而不是字符串数组，仓库层必须原样往返（含顺序）。
func TestProviderTemplateParamMappingRoundTrip(t *testing.T) {
	client, _ := integrationClient(t)
	institutionID := createInstitutionFixture(t, client, "provider-template-params")
	ctx := context.Background()

	providers := NewProviderRepository(client)
	providerID, err := providers.Create(ctx, &repository.Provider{
		InstitutionID: institutionID,
		Name:          "sms-primary",
		DisplayName:   "SMS primary",
		Category:      "sms",
		Type:          "aliyun",
		ClientID:      "access-key",
		ClientSecret:  "access-secret",
		SignName:      "Polaris",
		State:         "disabled",
	})
	if err != nil {
		t.Fatalf("create sms provider: %v", err)
	}

	templates := NewProviderTemplateRepository(client)
	item := &repository.ProviderTemplate{
		InstitutionID: institutionID,
		ProviderID:    providerID,
		Channel:       "sms",
		Name:          "login-code",
		Scene:         repository.TemplateSceneLoginCode,
		TemplateCode:  "SMS_1",
		TemplateParams: []repository.ProviderTemplateParam{
			{Key: "code1", Source: "code"},
			{Key: "minutes1", Source: "minutes"},
		},
		IsDefault: true,
	}
	templateID, err := templates.Create(ctx, item)
	if err != nil {
		t.Fatalf("create provider template: %v", err)
	}

	loaded, err := templates.GetByID(ctx, "sms", templateID)
	if err != nil {
		t.Fatalf("get provider template: %v", err)
	}
	if len(loaded.TemplateParams) != 2 ||
		loaded.TemplateParams[0] != (repository.ProviderTemplateParam{Key: "code1", Source: "code"}) ||
		loaded.TemplateParams[1] != (repository.ProviderTemplateParam{Key: "minutes1", Source: "minutes"}) {
		t.Fatalf("template param mapping did not round-trip: %+v", loaded.TemplateParams)
	}

	loaded.TemplateParams = []repository.ProviderTemplateParam{{Key: "code", Source: "code"}}
	if err := templates.Update(ctx, loaded); err != nil {
		t.Fatalf("update provider template: %v", err)
	}
	items, err := templates.ListByProvider(ctx, "sms", institutionID, providerID, repository.TemplateSceneLoginCode)
	if err != nil {
		t.Fatalf("list provider templates: %v", err)
	}
	if len(items) != 1 || len(items[0].TemplateParams) != 1 ||
		items[0].TemplateParams[0] != (repository.ProviderTemplateParam{Key: "code", Source: "code"}) {
		t.Fatalf("updated mapping not persisted: %+v", items)
	}
}

// 回归：邮件模板没有参数映射列，响应必须是空数组而不是 null（前端直接 .map 会崩）。
func TestEmailProviderTemplateNormalizesParams(t *testing.T) {
	client, _ := integrationClient(t)
	institutionID := createInstitutionFixture(t, client, "provider-email-template")
	ctx := context.Background()

	providers := NewProviderRepository(client)
	providerID, err := providers.Create(ctx, &repository.Provider{
		InstitutionID: institutionID,
		Name:          "mail-primary",
		DisplayName:   "Mail primary",
		Category:      "email",
		Type:          "smtp",
		Host:          "smtp.example.com",
		Port:          465,
		SslMode:       "auto",
		ClientID:      "noreply@example.com",
		ClientSecret:  "secret",
		FromAddress:   "Polaris <noreply@example.com>",
		State:         "active",
	})
	if err != nil {
		t.Fatalf("create email provider: %v", err)
	}

	templates := NewProviderTemplateRepository(client)
	templateID, err := templates.Create(ctx, &repository.ProviderTemplate{
		InstitutionID: institutionID,
		ProviderID:    providerID,
		Channel:       "email",
		Name:          "login-code",
		Scene:         repository.TemplateSceneLoginCode,
		Subject:       "验证码",
		Content:       "<p>您的验证码是 {{code}}</p>",
		IsDefault:     true,
	})
	if err != nil {
		t.Fatalf("create email template: %v", err)
	}

	loaded, err := templates.GetByID(ctx, "email", templateID)
	if err != nil {
		t.Fatalf("get email template: %v", err)
	}
	if loaded.TemplateParams == nil || len(loaded.TemplateParams) != 0 {
		t.Fatalf("email template params must be an empty slice, got %#v", loaded.TemplateParams)
	}
	items, err := templates.ListByProvider(ctx, "email", institutionID, providerID, repository.TemplateSceneLoginCode)
	if err != nil {
		t.Fatalf("list email templates: %v", err)
	}
	if len(items) != 1 || items[0].TemplateParams == nil {
		t.Fatalf("email template list params must be an empty slice: %#v", items)
	}
}
