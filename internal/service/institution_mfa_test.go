package service

import (
	"testing"

	"polaris/internal/repository"
)

// 回归：组织级 MFA 策略必须是受支持取值，记住时长不得为负（"配置即生效"前置校验）。
func TestValidatePasswordConfigMfaPolicy(t *testing.T) {
	cases := []struct {
		name    string
		org     *repository.Institution
		wantErr bool
	}{
		{"默认可选", &repository.Institution{MfaPolicy: repository.MfaPolicyOptional}, false},
		{"强制", &repository.Institution{MfaPolicy: repository.MfaPolicyRequired}, false},
		{"空值放行（沿用既有）", &repository.Institution{}, false},
		{"非法策略", &repository.Institution{MfaPolicy: "sometimes"}, true},
		{"记住时长为负", &repository.Institution{MfaPolicy: "optional", MfaRememberHours: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePasswordConfig(tc.org)
			if tc.wantErr && err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望校验通过，实际失败: %v", err)
			}
		})
	}
}
