package service

import (
	"strings"
	"testing"

	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

func TestTemplateChannelSceneConstraints(t *testing.T) {
	tests := []struct {
		name     string
		category string
		scene    string
		input    NotificationTemplateInput
		wantErr  bool
	}{
		{
			name: "email login", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneLoginCode,
			input: NotificationTemplateInput{Subject: "验证码", Content: "<p>您的验证码是 {{code}}</p>"},
		},
		{
			name: "email magic", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneMagicLink,
			input: NotificationTemplateInput{Subject: "登录链接", Content: `<p><a href="{{link}}">登录</a></p>`},
		},
		{
			name: "email mfa rejected", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneMfaCode,
			input: NotificationTemplateInput{Subject: "验证码", Content: "<p>{{code}}</p>"}, wantErr: true,
		},
		{
			name: "sms login", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneLoginCode,
			input: NotificationTemplateInput{
				TemplateCode:   "SMS_1",
				TemplateParams: []NotificationTemplateParam{{Key: "code", Source: "code"}},
			},
		},
		{
			name: "sms mfa", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneMfaCode,
			input: NotificationTemplateInput{
				TemplateCode:   "SMS_1",
				TemplateParams: []NotificationTemplateParam{{Key: "code", Source: "code"}},
			},
		},
		{
			name: "sms magic rejected", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneMagicLink,
			input: NotificationTemplateInput{
				TemplateCode:   "SMS_1",
				TemplateParams: []NotificationTemplateParam{{Key: "code", Source: "code"}},
			}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Name = "template"
			tt.input.Scene = tt.scene
			_, err := normalizeTemplateInput(tt.category, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeTemplateInput error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSTemplateParamsMapProviderKeysToPlaceholders(t *testing.T) {
	normalized, err := normalizeTemplateInput(string(providerpkg.CategorySMS), NotificationTemplateInput{
		Name: "登录验证码", Scene: repository.TemplateSceneLoginCode, TemplateCode: "SMS_1",
		TemplateParams: []NotificationTemplateParam{
			{Key: "code1", Source: "code"},
			{Key: "minutes1", Source: "minutes"},
		},
	})
	if err != nil {
		t.Fatalf("normalizeTemplateInput error = %v", err)
	}
	if len(normalized.TemplateParams) != 2 {
		t.Fatalf("参数映射丢失: %+v", normalized.TemplateParams)
	}
	if normalized.TemplateParams[0].Key != "code1" || normalized.TemplateParams[0].Source != "code" {
		t.Fatalf("厂商变量与平台占位符未分开保存: %+v", normalized.TemplateParams)
	}
}

func TestSMSTemplateParamsRequireCodeAndValidSource(t *testing.T) {
	tests := []struct {
		name   string
		params []NotificationTemplateParam
	}{
		{name: "missing code", params: []NotificationTemplateParam{{Key: "minutes1", Source: "minutes"}}},
		{name: "unknown source", params: []NotificationTemplateParam{{Key: "code1", Source: "unknown"}}},
		{name: "missing source", params: []NotificationTemplateParam{{Key: "code1"}}},
		{name: "duplicate key", params: []NotificationTemplateParam{
			{Key: "code1", Source: "code"}, {Key: "code1", Source: "minutes"},
		}},
		{name: "empty params", params: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeTemplateInput(string(providerpkg.CategorySMS), NotificationTemplateInput{
				Name: "登录验证码", Scene: repository.TemplateSceneLoginCode, TemplateCode: "SMS_1",
				TemplateParams: tt.params,
			})
			if err == nil {
				t.Fatal("非法参数映射应被拒绝")
			}
		})
	}
}

func TestEmailTemplateScenePlaceholdersAreScoped(t *testing.T) {
	_, err := normalizeTemplateInput(string(providerpkg.CategoryEmail), NotificationTemplateInput{
		Name: "登录验证码", Scene: repository.TemplateSceneLoginCode,
		Content: `<p>您的验证码是 {{code}}</p><p><a href="{{link}}">登录</a></p>`,
	})
	if err == nil {
		t.Fatal("登录验证码场景不应接受魔法链接占位符")
	}
	_, err = normalizeTemplateInput(string(providerpkg.CategoryEmail), NotificationTemplateInput{
		Name: "魔法链接", Scene: repository.TemplateSceneMagicLink, Content: "<p>请点击邮件中的链接</p>",
	})
	if err == nil {
		t.Fatal("魔法链接场景必须引用 {{link}}")
	}
	_, err = normalizeTemplateInput(string(providerpkg.CategoryEmail), NotificationTemplateInput{
		Name: "登录验证码", Scene: repository.TemplateSceneLoginCode, Content: "<p>请查收验证码</p>",
	})
	if err == nil {
		t.Fatal("登录验证码场景必须引用 {{code}}")
	}
}

func TestEmailTemplateRichTextSanitization(t *testing.T) {
	normalized, err := normalizeTemplateInput(string(providerpkg.CategoryEmail), NotificationTemplateInput{
		Name: "登录验证码", Scene: repository.TemplateSceneLoginCode,
		Content: `<p onclick="steal()">您的验证码是 {{code}}<script>alert(1)</script>` +
			`<a href="javascript:alert(1)">坏链接</a>` +
			`<a href="https://polaris.example.com/login?a=1&b=2">好链接</a></p>`,
	})
	if err != nil {
		t.Fatalf("normalizeTemplateInput error = %v", err)
	}
	for _, forbidden := range []string{"<script", "onclick", "javascript:"} {
		if strings.Contains(strings.ToLower(normalized.Content), forbidden) {
			t.Fatalf("清洗后仍包含危险内容 %q: %s", forbidden, normalized.Content)
		}
	}
	if !strings.Contains(normalized.Content, `href="https://polaris.example.com/login?a=1&amp;b=2"`) {
		t.Fatalf("安全链接被误删: %s", normalized.Content)
	}
}

func TestEmailLegacyPlainTextBecomesHTML(t *testing.T) {
	normalized, err := normalizeTemplateInput(string(providerpkg.CategoryEmail), NotificationTemplateInput{
		Name: "登录验证码", Scene: repository.TemplateSceneLoginCode,
		Content: "您的验证码是 {{code}}\n5 分钟内有效",
	})
	if err != nil {
		t.Fatalf("normalizeTemplateInput error = %v", err)
	}
	if !strings.Contains(normalized.Content, "<p>您的验证码是 {{code}}<br>5 分钟内有效</p>") {
		t.Fatalf("历史纯文本未转换为段落: %s", normalized.Content)
	}
}

func TestTemplateChannelForMethod(t *testing.T) {
	tests := map[string]string{
		repository.SigninMethodEmailCode: "email",
		repository.SigninMethodMagicLink: "email",
		repository.SigninMethodSMSCode:   "sms",
		repository.SigninMethodPassword:  "",
	}
	for method, want := range tests {
		if got := channelForMethod(method); got != want {
			t.Fatalf("channelForMethod(%s) = %q, want %q", method, got, want)
		}
	}
}
