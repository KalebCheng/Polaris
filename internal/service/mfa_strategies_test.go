package service

import (
	"testing"

	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

func TestMFAStrategyRegistry(t *testing.T) {
	for _, kind := range []string{providerpkg.TypeMFATOTP, providerpkg.TypeMFASMS} {
		provider := &repository.Provider{Category: "mfa", Type: kind}
		strategy, err := mfaStrategyFor(provider)
		if err != nil || strategy == nil {
			t.Fatalf("MFA 类型 %s 缺少策略: %v", kind, err)
		}
	}
}

func TestCodeChannelStrategyRegistry(t *testing.T) {
	for _, kind := range []CodeKind{CodeKindEmail, CodeKindSMS} {
		strategy, ok := codeChannelStrategies[kind]
		if !ok || strategy.signinMethod == "" || strategy.category == "" {
			t.Fatalf("验证码通道 %s 缺少策略", kind)
		}
	}
}
