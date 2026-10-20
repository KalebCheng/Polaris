package service

import (
	"testing"

	"polaris/internal/auth/oauth"
	providerpkg "polaris/internal/service/provider"
)

func TestMatchProviderValueStrategies(t *testing.T) {
	info := &oauth.UserInfo{
		ProviderUserID: "provider-id",
		UnionID:        "union-id",
		Email:          "user@example.com",
		Phone:          "13800000000",
		DisplayName:    "成凯",
		Username:       "kcheng",
	}
	cases := map[string]string{
		"unionid":            "union-id",
		"union_id":           "union-id",
		"openid":             "provider-id",
		"open_id":            "provider-id",
		"sub":                "provider-id",
		"email":              "user@example.com",
		"mobile":             "13800000000",
		"phone_number":       "13800000000",
		"nick":               "成凯",
		"nickname":           "成凯",
		"name":               "成凯",
		"preferred_username": "kcheng",
		"  EMAIL  ":          "user@example.com",
	}
	for field, want := range cases {
		if got := matchProviderValue(info, field); got != want {
			t.Fatalf("matchProviderValue(%q) = %q, want %q", field, got, want)
		}
	}
	if got := matchProviderValue(info, "not-a-field"); got != "" {
		t.Fatalf("未知字段应返回空串，实际 %q", got)
	}
	if got := matchProviderValue(nil, "email"); got != "" {
		t.Fatalf("nil UserInfo 应返回空串，实际 %q", got)
	}
}

// 目录里声明的每个提供商匹配字段都必须有运行时提取策略，避免「可选但取不到值」。
func TestCatalogProviderMatchFieldsHaveExtractor(t *testing.T) {
	for _, category := range providerpkg.Categories() {
		if string(category.Key) != string(providerpkg.CategoryOAuth) {
			continue
		}
		for _, info := range category.Types {
			field, ok := providerpkg.LookupField(info, "match_provider_field")
			if !ok {
				t.Fatalf("类型 %s 缺少 match_provider_field", info.Key)
			}
			for _, option := range field.Options {
				if option.Value == "" {
					continue
				}
				if _, ok := providerFieldStrategies[option.Value]; !ok {
					t.Fatalf("类型 %s 的匹配字段 %s 没有提取策略", info.Key, option.Value)
				}
			}
		}
	}
}
