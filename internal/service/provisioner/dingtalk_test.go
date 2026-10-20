package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDingTalk 起一个假钉钉开放平台：校验凭据换 token，并记录通讯录接口调用。
func fakeDingTalk(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gettoken":
			if r.URL.Query().Get("appkey") != "app-key" || r.URL.Query().Get("appsecret") != "app-secret" {
				_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid credential"}`))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"access_token":"tk-1"}`))
		default:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.URL.Query().Get("access_token") != "tk-1" {
				_, _ = w.Write([]byte(`{"errcode":40014,"errmsg":"invalid access_token"}`))
				return
			}
			if body["userid"] != "alice" {
				_, _ = w.Write([]byte(`{"errcode":40035,"errmsg":"userid required"}`))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func useFakeDingTalk(t *testing.T) *[]string {
	t.Helper()
	server, calls := fakeDingTalk(t)
	previous := dingTalkBase
	dingTalkBase = server.URL
	t.Cleanup(func() { dingTalkBase = previous })
	return calls
}

func dingTalkConfig() Config {
	return Config{Type: "dingtalk", Values: map[string]string{
		"app_id": "app-key", "app_secret": "app-secret", "dept_id": "42",
	}}
}

// 回归：能力声明必须与实现一致——钉钉是 SSO 场景（无口令）、通讯录没有"停用态"。
func TestDingTalkCapabilities(t *testing.T) {
	caps := (&DingTalkProvisioner{}).Capabilities(dingTalkConfig())
	if caps.Password {
		t.Fatal("钉钉不应声明支持口令（SSO 场景）")
	}
	if caps.Disable {
		t.Fatal("钉钉通讯录没有停用态，不应声明支持停用")
	}
	if !caps.Update || !caps.Delete {
		t.Fatal("钉钉应支持更新与删除")
	}
	// 停用必须明确返回 ErrUnsupported（由服务层转成 skipped，而不是重试）
	err := (&DingTalkProvisioner{}).Disable(context.Background(), dingTalkConfig(), &Account{Username: "alice"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("停用应返回 ErrUnsupported，实际 %v", err)
	}
}

// 回归：建号/更新/删除走正确的接口，并把凭据与部门带过去。
func TestDingTalkProvisionFlow(t *testing.T) {
	calls := useFakeDingTalk(t)
	prov := &DingTalkProvisioner{}
	cfg := dingTalkConfig()

	username, err := prov.Provision(context.Background(), cfg, &Account{
		Username: "alice", DisplayName: "爱丽丝", Phone: "13800000000", Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if username != "alice" {
		t.Fatalf("建号应返回本地账号名，实际 %q", username)
	}
	if err := prov.Update(context.Background(), cfg, &Account{Username: "alice", DisplayName: "Alice"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if err := prov.Delete(context.Background(), cfg, &Account{Username: "alice"}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	joined := strings.Join(*calls, ",")
	for _, want := range []string{"/gettoken", "/topapi/v2/user/create", "/topapi/v2/user/update", "/topapi/v2/user/delete"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("未调用 %s（实际调用 %s）", want, joined)
		}
	}
}

// 回归：凭据错误必须如实上报（不能"看起来成功"）。
func TestDingTalkRejectsBadCredential(t *testing.T) {
	useFakeDingTalk(t)
	prov := &DingTalkProvisioner{}
	err := prov.Ping(context.Background(), Config{Type: "dingtalk", Values: map[string]string{
		"app_id": "app-key", "app_secret": "wrong",
	}})
	if err == nil {
		t.Fatal("错误的 AppSecret 应报错")
	}
	if err := prov.Ping(context.Background(), Config{Type: "dingtalk"}); err == nil {
		t.Fatal("缺少凭据应报错")
	}
}

// 回归：Config.Value 的兼容读取（Values 优先，其次旧的扁平字段）
func TestConfigValue(t *testing.T) {
	cfg := Config{Values: map[string]string{"provider_url": "https://a"}, ProviderURL: "https://b"}
	if got := cfg.Value("provider_url"); got != "https://a" {
		t.Fatalf("Values 应优先，实际 %q", got)
	}
	legacy := Config{ProviderURL: "https://b", Principal: "svc", Credentials: "token"}
	if legacy.Value("provider_url") != "https://b" || legacy.Value("principal") != "svc" || legacy.Value("credentials") != "token" {
		t.Fatal("旧字段应可回退读取")
	}
}
