package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakePerUser 起一个按用户名返回不同结果的假下游（用于存在性判定的三种结果）
func fakePerUser(t *testing.T, token string, handler func(username string, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		username := r.FormValue("userid")
		if username == "" {
			// 多数厂商把 userid 放在 JSON 请求体里
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if value, ok := body["userid"].(string); ok {
				username = value
			}
		}
		if username == "" {
			username = r.URL.Path
		}
		if token != "" && r.URL.Query().Get("access_token") == "" && r.Method != http.MethodPost {
			_, _ = w.Write([]byte(`{"errcode":0,"access_token":"` + token + `"}`))
			return
		}
		handler(username, w)
	}))
	t.Cleanup(server.Close)
	return server
}

// 回归：钉钉存在性判定——只有 60121（用户不存在）才算"缺失"，其他错误码必须归为"无法判定"。
func TestDingTalkExistsUser(t *testing.T) {
	server := fakePerUser(t, "tk", func(username string, w http.ResponseWriter) {
		switch username {
		case "exists-user":
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		case "missing-user":
			_, _ = w.Write([]byte(`{"errcode":60121,"errmsg":"user not exist"}`))
		default:
			_, _ = w.Write([]byte(`{"errcode":88,"errmsg":"no permission"}`))
		}
	})
	previous := dingTalkBase
	dingTalkBase = server.URL
	t.Cleanup(func() { dingTalkBase = previous })

	prov := &DingTalkProvisioner{}
	cfg := Config{Type: "dingtalk", Values: map[string]string{"app_id": "k", "app_secret": "s"}}
	cases := map[string]ExistsState{"exists-user": ExistsYes, "missing-user": ExistsNo, "other-user": ExistsUnknown}
	for username, want := range cases {
		result, err := prov.ExistsUser(context.Background(), cfg, username)
		if err != nil {
			t.Fatalf("%s 判定失败: %v", username, err)
		}
		if result.State != want {
			t.Errorf("%s 期望 %s，实际 %s", username, want, result.State)
		}
		if want == ExistsUnknown && result.Detail == "" {
			t.Errorf("%s 无法判定时必须带上原因（含下游错误码）", username)
		}
	}
}

// 回归：企业微信存在性判定（60111 = 成员不存在）
func TestWeComExistsUser(t *testing.T) {
	server := fakePerUser(t, "wt", func(username string, w http.ResponseWriter) {
		switch username {
		case "missing-user":
			_, _ = w.Write([]byte(`{"errcode":60111,"errmsg":"invalid userid"}`))
		default:
			_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
		}
	})
	previous := wecomBase
	wecomBase = server.URL
	t.Cleanup(func() { wecomBase = previous })

	prov := &WeComProvisioner{}
	cfg := Config{Type: "wecom", Values: map[string]string{"app_id": "c", "app_secret": "s"}}
	if result, _ := prov.ExistsUser(context.Background(), cfg, "alice"); result.State != ExistsYes {
		t.Fatalf("存在账号应判定为存在，实际 %s", result.State)
	}
	if result, _ := prov.ExistsUser(context.Background(), cfg, "missing-user"); result.State != ExistsNo {
		t.Fatalf("60111 应判定为缺失，实际 %s", result.State)
	}
}

// 回归：通用 HTTP 契约——GET 2xx = 存在，404 = 缺失，其余状态码 = 无法判定
func TestHTTPExistsUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/accounts/exists-user":
			w.WriteHeader(http.StatusOK)
		case "/accounts/missing-user":
			w.WriteHeader(http.StatusNotFound)
		case "/accounts/forbidden-user":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	prov := &HTTPProvisioner{}
	cfg := Config{Type: "http", Values: map[string]string{"provider_url": server.URL}}
	cases := map[string]ExistsState{
		"exists-user": ExistsYes, "missing-user": ExistsNo, "forbidden-user": ExistsUnknown,
	}
	for username, want := range cases {
		result, err := prov.ExistsUser(context.Background(), cfg, username)
		if err != nil {
			t.Fatalf("%s 判定失败: %v", username, err)
		}
		if result.State != want {
			t.Errorf("%s 期望 %s，实际 %s（%s）", username, want, result.State, result.Detail)
		}
	}
	// 未配置 provider_url 时必须报错（而不是静默"不存在"）
	if _, err := prov.ExistsUser(context.Background(), Config{Type: "http"}, "alice"); err == nil {
		t.Fatal("缺少 provider_url 应报错")
	}
}

// 回归：LDAP 存在性过滤器的转义（避免 LDAP 注入）
func TestLDAPExistsFilterEscaping(t *testing.T) {
	if got := existsUserFilter("uid", "smith*"); got != `(uid=smith\2a)` {
		t.Fatalf("通配符应被转义，实际 %s", got)
	}
	if got := existsUserFilter("cn", "a(b)c"); got != `(cn=a\28b\29c)` {
		t.Fatalf("括号应被转义，实际 %s", got)
	}
}
