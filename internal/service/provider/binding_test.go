package provider

import (
	"testing"

	"polaris/internal/repository"
)

func oauthInfo(t *testing.T) TypeInfo {
	t.Helper()
	info, ok := Find(CategoryOAuth, TypeOIDC)
	if !ok {
		t.Fatal("目录缺少 oauth/oidc")
	}
	return info
}

func TestBindRejectsUnknownAndMisplacedKeys(t *testing.T) {
	info := oauthInfo(t)
	cases := []struct {
		name    string
		config  map[string]any
		policy  map[string]any
		secrets map[string]string
	}{
		{"未知 config 键", map[string]any{"not_a_field": "x"}, nil, nil},
		{"敏感键混入 config", map[string]any{"client_secret": "plain"}, nil, nil},
		{"策略键混入 config", map[string]any{"auto_create": true}, nil, nil},
		{"未知 policy 键", nil, map[string]any{"whatever": "x"}, nil},
		{"非策略键放进 policy", nil, map[string]any{"client_id": "x"}, nil},
		{"未知 secrets 键", nil, nil, map[string]string{"nope": "x"}},
		{"非敏感键放进 secrets", nil, nil, map[string]string{"client_id": "x"}},
	}
	for _, tc := range cases {
		item := &repository.Provider{}
		if err := Bind(item, info, tc.config, tc.policy, tc.secrets, BindOptions{}); err == nil {
			t.Fatalf("%s：应当被拒绝", tc.name)
		}
	}
}

func TestBindMapsBucketsToColumns(t *testing.T) {
	info := oauthInfo(t)
	item := &repository.Provider{HasSecret: true}
	err := Bind(item, info,
		map[string]any{
			"name":         "corp-sso",
			"display_name": "企业单点登录",
			"issuer_url":   "https://idp.example.com",
			"client_id":    "polaris",
			"scopes":       []any{"openid", "profile"},
			"enable_pkce":  "true",
			"state":        "active",
		},
		map[string]any{"auto_create": true, "link_by_email": false, "allowed_domains": []any{"corp.example.com"}},
		map[string]string{"client_secret": "s3cr3t"},
		BindOptions{})
	if err != nil {
		t.Fatalf("绑定失败: %v", err)
	}
	if item.ClientID != "polaris" || item.ClientSecret != "s3cr3t" {
		t.Fatalf("凭据未落库: %+v", item)
	}
	if item.IssuerURL != "https://idp.example.com" {
		t.Fatalf("issuer_url 未落库: %s", item.IssuerURL)
	}
	if len(item.Scopes) != 2 || item.Scopes[0] != "openid" {
		t.Fatalf("scopes 未落库: %+v", item.Scopes)
	}
	if !item.EnablePKCE {
		t.Fatal("enable_pkce 未落库")
	}
	if !item.AutoCreate || item.LinkByEmail {
		t.Fatalf("信任策略未落库: auto_create=%v link_by_email=%v", item.AutoCreate, item.LinkByEmail)
	}
	if len(item.AllowedDomains) != 1 || item.AllowedDomains[0] != "corp.example.com" {
		t.Fatalf("allowed_domains 未落库: %+v", item.AllowedDomains)
	}
}

func TestBindRequiredFields(t *testing.T) {
	info := oauthInfo(t)
	item := &repository.Provider{}
	if err := Bind(item, info, map[string]any{"client_id": "x"}, nil, nil, BindOptions{}); err == nil {
		t.Fatal("缺少必填的 issuer_url 与 client_secret 时应拒绝创建")
	}
	// 编辑态：敏感字段留空表示保持原值，必填校验不再拦
	item = &repository.Provider{HasSecret: true}
	err := Bind(item, info,
		map[string]any{"name": "corp", "issuer_url": "https://idp.example.com", "client_id": "x"},
		nil, nil, BindOptions{KeepSecretsOnEmpty: true})
	if err != nil {
		t.Fatalf("编辑态留空密钥不应被必填校验拒绝: %v", err)
	}
}

func TestExtractSkipsSecretsAndSplitsPolicy(t *testing.T) {
	info := oauthInfo(t)
	item := &repository.Provider{
		ClientID: "polaris", ClientSecret: "ciphertext", IssuerURL: "https://idp.example.com",
		AutoCreate: true, LinkByEmail: true, Scopes: []string{"openid"},
	}
	config, policy := Extract(item, info)
	if _, leaked := config["client_secret"]; leaked {
		t.Fatal("config 不得包含敏感字段")
	}
	if config["client_id"] != "polaris" {
		t.Fatalf("config 缺少 client_id: %+v", config)
	}
	if _, misplaced := config["auto_create"]; misplaced {
		t.Fatal("策略字段不得出现在 config 桶")
	}
	if policy["auto_create"] != true || policy["link_by_email"] != true {
		t.Fatalf("policy 桶不正确: %+v", policy)
	}
	// 一级字段（name/display_name/state）不重复进 config
	for _, key := range []string{"name", "display_name", "state"} {
		if _, dup := config[key]; dup {
			t.Fatalf("config 不应重复承载一级字段 %s", key)
		}
	}
}

func TestKindNormalizesLegacyRows(t *testing.T) {
	// 迁移前的历史行：类别大写、recaptcha 子类型、类型留空
	cases := []struct {
		item *repository.Provider
		want string
	}{
		{&repository.Provider{Category: "Captcha", Type: "recaptcha_v2"}, "recaptcha"},
		{&repository.Provider{Category: "OAuth", Type: "DingTalk"}, "dingtalk"},
		{&repository.Provider{Category: "storage", Type: ""}, "local"},
	}
	for _, tc := range cases {
		if got := Kind(tc.item); got != tc.want {
			t.Fatalf("Kind(%+v) = %s, want %s", tc.item, got, tc.want)
		}
	}
}
