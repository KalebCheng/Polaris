package service

import (
	"testing"

	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

func TestTemplateChannelSceneConstraints(t *testing.T) {
	tests := []struct {
		name     string
		category string
		scene    string
		wantErr  bool
	}{
		{name: "email login", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneLoginCode},
		{name: "email magic", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneMagicLink},
		{name: "email mfa rejected", category: string(providerpkg.CategoryEmail), scene: repository.TemplateSceneMfaCode, wantErr: true},
		{name: "sms login", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneLoginCode},
		{name: "sms mfa", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneMfaCode},
		{name: "sms magic rejected", category: string(providerpkg.CategorySMS), scene: repository.TemplateSceneMagicLink, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeTemplateInput(tt.category, NotificationTemplateInput{
				Name: "template", Scene: tt.scene, TemplateCode: "SMS_1", Subject: "subject", Content: "body {{code}}",
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeTemplateInput error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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
