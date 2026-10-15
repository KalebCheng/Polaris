package provisioner

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归：下游连接器必须按统一契约发请求（路径 / 方法 / Basic 鉴权 / 建号返回下游账号名）。
func TestHTTPProvisionerContract(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("svc:secret"))
		if r.Header.Get("Authorization") != wantAuth {
			t.Errorf("鉴权头错误: %q", r.Header.Get("Authorization"))
		}
		got = append(got, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/accounts" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"downstream-alice"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := Default()
	if p == nil || p.Type() != "http" {
		t.Fatalf("默认连接器未注册: %v", p)
	}
	cfg := Config{ProviderURL: srv.URL + "/", Principal: "svc", Credentials: "secret"}
	account := &Account{Username: "alice", Email: "alice@example.com", Password: "P@ssw0rd"}

	name, err := p.Provision(context.Background(), cfg, account)
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if name != "downstream-alice" {
		t.Fatalf("应以响应体 username 为准，实际 %q", name)
	}
	account.Username = name
	if err := p.Update(context.Background(), cfg, account); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if err := p.Disable(context.Background(), cfg, account); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if err := p.Enable(context.Background(), cfg, account); err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	if err := p.Delete(context.Background(), cfg, account); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	want := []string{
		"POST /accounts",
		"PUT /accounts/downstream-alice",
		"POST /accounts/downstream-alice/disable",
		"POST /accounts/downstream-alice/enable",
		"DELETE /accounts/downstream-alice",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("契约路径不符:\n got  %v\n want %v", got, want)
	}
}

// 回归：下游返回非 2xx 必须报错，不得伪装成功。
func TestHTTPProvisionerSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"duplicate"}`))
	}))
	defer srv.Close()
	p := Default()
	cfg := Config{ProviderURL: srv.URL}
	if _, err := p.Provision(context.Background(), cfg, &Account{Username: "x"}); err == nil {
		t.Fatal("下游 409 应报错")
	}
}

// 回归：未配置 provider_url 时建号必须直接失败。
func TestHTTPProvisionerRequiresURL(t *testing.T) {
	if _, err := Default().Provision(context.Background(), Config{}, &Account{Username: "x"}); err == nil {
		t.Fatal("缺少 provider_url 应报错")
	}
}

// 账号名中的保留字符必须转义，避免路径注入。
func TestPathEscape(t *testing.T) {
	if got := pathEscape("a/b c"); got != "a%2Fb%20c" {
		t.Fatalf("pathEscape 转义错误: %q", got)
	}
}

func TestRegistryHasHTTP(t *testing.T) {
	if _, err := Get("http"); err != nil {
		t.Fatalf("内置连接器 http 未注册: %v", err)
	}
	if _, err := Get("nope"); err == nil {
		t.Fatal("未注册类型应报错")
	}
}
