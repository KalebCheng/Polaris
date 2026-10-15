package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeWeCom 假企业微信接口：校验 CorpID/Secret 换 token，并记录通讯录调用。
func fakeWeCom(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/cgi-bin/gettoken" {
			if r.URL.Query().Get("corpid") != "corp-1" || r.URL.Query().Get("corpsecret") != "sec-1" {
				_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid corpid or secret"}`))
				return
			}
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"wt-1"}`))
			return
		}
		if r.URL.Query().Get("access_token") != "wt-1" {
			_, _ = w.Write([]byte(`{"errcode":40014,"errmsg":"invalid access_token"}`))
			return
		}
		// 建号必须带部门（企业微信要求）
		if r.URL.Path == "/cgi-bin/user/create" && r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["department"]; !ok {
				_, _ = w.Write([]byte(`{"errcode":60001,"errmsg":"department required"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(server.Close)
	previous := wecomBase
	wecomBase = server.URL
	t.Cleanup(func() { wecomBase = previous })
	return &calls
}

func wecomConfig() Config {
	return Config{Type: "wecom", Values: map[string]string{
		"app_id": "corp-1", "app_secret": "sec-1",
	}}
}

// 回归（依据官方文档核对）：企业微信无独立口令，但**支持启用/停用成员**——
// 「更新成员」接口的 `enable` 字段（1 启用、0 禁用）。
func TestWeComCapabilities(t *testing.T) {
	caps := (&WeComProvisioner{}).Capabilities(wecomConfig())
	if caps.Password {
		t.Fatal("企业微信不应声明支持口令（账号在企微侧管理）")
	}
	if !caps.Disable {
		t.Fatal("官方 `user/update` 提供 enable 字段，应声明支持停用")
	}
	if !caps.Update || !caps.Delete {
		t.Fatal("企业微信应支持更新与删除")
	}
}

// 回归：停用/启用走 `user/update` 且携带官方 enable 字段（0 禁用 / 1 启用）
func TestWeComEnableDisable(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/cgi-bin/gettoken" {
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok","access_token":"wt-1"}`))
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(server.Close)
	previous := wecomBase
	wecomBase = server.URL
	t.Cleanup(func() { wecomBase = previous })

	prov := &WeComProvisioner{}
	cfg := wecomConfig()
	if err := prov.Disable(context.Background(), cfg, &Account{Username: "alice"}); err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if err := prov.Enable(context.Background(), cfg, &Account{Username: "alice"}); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("应产生两次更新请求，实际 %d", len(bodies))
	}
	if bodies[0]["enable"] != float64(0) || bodies[0]["userid"] != "alice" {
		t.Fatalf("停用应携带 enable=0: %+v", bodies[0])
	}
	if bodies[1]["enable"] != float64(1) {
		t.Fatalf("启用应携带 enable=1: %+v", bodies[1])
	}
}

// 回归：建号必须带部门（未配置时归入根部门 1）；删除走 GET + userid。
func TestWeComProvisionFlow(t *testing.T) {
	calls := fakeWeCom(t)
	prov := &WeComProvisioner{}
	cfg := wecomConfig()

	if _, err := prov.Provision(context.Background(), cfg, &Account{
		Username: "alice", DisplayName: "爱丽丝", Phone: "13800000000", Email: "alice@example.com",
	}); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if err := prov.Update(context.Background(), cfg, &Account{Username: "alice", DisplayName: "Alice"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if err := prov.Delete(context.Background(), cfg, &Account{Username: "alice"}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	joined := strings.Join(*calls, ",")
	for _, want := range []string{
		"GET /cgi-bin/gettoken",
		"POST /cgi-bin/user/create",
		"POST /cgi-bin/user/update",
		"GET /cgi-bin/user/delete",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("未调用 %s（实际调用 %s）", want, joined)
		}
	}
}

// 回归：显式配置部门时应使用该部门
func TestWeComDepartmentFallback(t *testing.T) {
	if got := wecomDepartment(wecomConfig()); len(got) != 1 || got[0] != 1 {
		t.Fatalf("未配置部门应归入根部门 1，实际 %v", got)
	}
	cfg := wecomConfig()
	cfg.Values["dept_id"] = "7"
	if got := wecomDepartment(cfg); len(got) != 1 || got[0] != 7 {
		t.Fatalf("应使用配置的部门，实际 %v", got)
	}
}

// 回归：凭据错误必须如实上报
func TestWeComRejectsBadCredential(t *testing.T) {
	fakeWeCom(t)
	prov := &WeComProvisioner{}
	if err := prov.Ping(context.Background(), Config{Type: "wecom", Values: map[string]string{
		"app_id": "corp-1", "app_secret": "wrong",
	}}); err == nil {
		t.Fatal("错误的 Secret 应报错")
	}
	if err := prov.Ping(context.Background(), Config{Type: "wecom"}); err == nil {
		t.Fatal("缺少凭据应报错")
	}
}
