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

// fakeFeishu 假飞书开放平台：校验应用凭据换 token，并记录通讯录接口调用。
func fakeFeishu(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["app_id"] != "cli-app" || body["app_secret"] != "cli-secret" {
				_, _ = w.Write([]byte(`{"code":10003,"msg":"invalid app credentials"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-1"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer t-1" {
			_, _ = w.Write([]byte(`{"code":99991663,"msg":"invalid access token"}`))
			return
		}
		if !strings.Contains(r.URL.Path, "/contact/v3/users") {
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	t.Cleanup(server.Close)
	previous := feishuBase
	feishuBase = server.URL
	t.Cleanup(func() { feishuBase = previous })
	return &calls
}

func feishuConfig() Config {
	return Config{Type: "lark", Values: map[string]string{
		"app_id": "cli-app", "app_secret": "cli-secret", "dept_id": "od_1",
	}}
}

// 回归：能力声明与实现一致——飞书账号无独立口令、通讯录没有停用态。
func TestFeishuCapabilities(t *testing.T) {
	caps := (&FeishuProvisioner{}).Capabilities(feishuConfig())
	if caps.Password || caps.Disable {
		t.Fatalf("飞书不应声明支持口令或停用: %+v", caps)
	}
	if !caps.Update || !caps.Delete {
		t.Fatal("飞书应支持更新与删除")
	}
	if err := (&FeishuProvisioner{}).Disable(context.Background(), feishuConfig(), &Account{Username: "alice"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("停用应返回 ErrUnsupported，实际 %v", err)
	}
}

// 回归：建号/更新/删除走正确的方法与路径（user_id 为本地账号名）
func TestFeishuProvisionFlow(t *testing.T) {
	calls := fakeFeishu(t)
	prov := &FeishuProvisioner{}
	cfg := feishuConfig()

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
	for _, want := range []string{
		"POST /open-apis/auth/v3/tenant_access_token/internal",
		"POST /open-apis/contact/v3/users",
		"PUT /open-apis/contact/v3/users/alice",
		"DELETE /open-apis/contact/v3/users/alice",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("未调用 %s（实际调用 %s）", want, joined)
		}
	}
}

// 回归：凭据错误必须如实上报
func TestFeishuRejectsBadCredential(t *testing.T) {
	fakeFeishu(t)
	prov := &FeishuProvisioner{}
	if err := prov.Ping(context.Background(), Config{Type: "lark", Values: map[string]string{
		"app_id": "cli-app", "app_secret": "wrong",
	}}); err == nil {
		t.Fatal("错误的 App Secret 应报错")
	}
	if err := prov.Ping(context.Background(), Config{Type: "lark"}); err == nil {
		t.Fatal("缺少凭据应报错")
	}
}

// 回归：国际版 Lark 通过 base_url 覆盖开放平台域名
func TestFeishuCustomBaseURL(t *testing.T) {
	calls := fakeFeishu(t)
	cfg := feishuConfig()
	cfg.Values["base_url"] = feishuBase // 指向假服务器（等价于把域名换成 open.larksuite.com）
	if err := (&FeishuProvisioner{}).Ping(context.Background(), cfg); err != nil {
		t.Fatalf("自定义域名自检失败: %v", err)
	}
	if len(*calls) == 0 {
		t.Fatal("应调用 token 接口")
	}
}
