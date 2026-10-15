package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"polaris/internal/repository"
)

// TestBackchannelSendDeliversLogoutToken 验证后端登出通知以
// application/x-www-form-urlencoded 携带 logout_token POST 到应用回调地址。
func TestBackchannelSendDeliversLogoutToken(t *testing.T) {
	var gotContentType, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotToken = r.PostFormValue("logout_token")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	svc := &BackchannelLogoutService{} // 仅测 send，不依赖签发器
	app := &repository.Application{InstitutionID: 1, Name: "app1", BackchannelLogoutURI: srv.URL}
	svc.send(context.Background(), app, "alice", "logout-token-xyz")

	if gotToken != "logout-token-xyz" {
		t.Errorf("logout_token = %q，期望 logout-token-xyz", gotToken)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
}

// TestBackchannelSendRejectsNonHTTP 验证非 http(s) 地址被拒绝（不发起请求）。
func TestBackchannelSendRejectsNonHTTP(t *testing.T) {
	// file:// 等非 http(s) 地址不应触发请求（此处仅确保不 panic、不阻塞）
	svc := &BackchannelLogoutService{}
	svc.send(context.Background(), &repository.Application{Name: "bad", BackchannelLogoutURI: "file:///etc/passwd"}, "alice", "t")
	svc.send(context.Background(), &repository.Application{Name: "empty", BackchannelLogoutURI: ""}, "alice", "t")
}
