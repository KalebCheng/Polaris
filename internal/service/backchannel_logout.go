package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"polaris/internal/auth/oidc"
	"polaris/internal/logger"
	"polaris/internal/repository"
)

// backchannelHTTPClient 后端登出（back-channel logout）通知的下游 HTTP 客户端。
var backchannelHTTPClient = &http.Client{Timeout: 10 * time.Second}

// BackchannelLogoutService 实现 OpenID Connect Back-Channel Logout：
// 本地会话结束时，向"该用户在各应用下、配置了 backchannel_logout_uri 的应用"发送 logout_token，
// 使下游 RP 同步注销其本地会话。
//
// 依据：docs/specs/2610101436-tenant-rearchitecture-and-maxkey-borrowing.md §3.5(b)。
type BackchannelLogoutService struct {
	apps   repository.ApplicationRepository
	tokens repository.TokenRepository
	issuer *oidc.Issuer
	audits *AuditRecorder
}

// NewBackchannelLogoutService 创建后端登出服务
func NewBackchannelLogoutService(apps repository.ApplicationRepository, tokens repository.TokenRepository, issuer *oidc.Issuer, audits *AuditRecorder) *BackchannelLogoutService {
	return &BackchannelLogoutService{apps: apps, tokens: tokens, issuer: issuer, audits: audits}
}

// NotifySessionEnd 在本地会话结束时异步通知下游应用。
// 该方法不阻塞调用方：逐个应用发送 logout_token，失败仅记审计与日志。
func (s *BackchannelLogoutService) NotifySessionEnd(ctx context.Context, institutionID int64, userName, subject, sessionID string) {
	if s == nil || s.issuer == nil || institutionID == 0 {
		return
	}
	apps := s.appsForUser(ctx, institutionID, userName)
	if len(apps) == 0 {
		return
	}
	for _, app := range apps {
		token, _, err := s.issuer.IssueLogoutToken(oidc.LogoutTokenParams{
			Subject: subject, Audience: app.ClientID, SessionID: sessionID,
		})
		if err != nil {
			logger.Warn("签发 logout_token 失败", "application", app.Code, "error", err)
			continue
		}
		s.send(ctx, app, userName, token)
	}
}

// appsForUser 返回该用户有活跃令牌、且配置了 backchannel_logout_uri 的启用应用。
func (s *BackchannelLogoutService) appsForUser(ctx context.Context, institutionID int64, userName string) []*repository.Application {
	recs, err := s.tokens.List(ctx, repository.ListQuery{InstitutionID: institutionID, Field: "user", Value: userName, Limit: 200})
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var apps []*repository.Application
	for _, rec := range recs {
		if rec.Application == "" || seen[rec.Application] {
			continue
		}
		seen[rec.Application] = true
		app, err := s.apps.GetByCode(ctx, institutionID, rec.Application)
		if err != nil || app.State != "active" || strings.TrimSpace(app.BackchannelLogoutURI) == "" {
			continue
		}
		apps = append(apps, app)
	}
	return apps
}

// send 向单个应用发送 logout_token（application/x-www-form-urlencoded，参数名 logout_token）。
func (s *BackchannelLogoutService) send(ctx context.Context, app *repository.Application, userName, logoutToken string) {
	endpoint := strings.TrimSpace(app.BackchannelLogoutURI)
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		logger.Warn("非法的 backchannel_logout_uri，跳过", "application", app.Code, "uri", endpoint)
		return
	}
	form := url.Values{}
	form.Set("logout_token", logoutToken)
	body := bytes.NewBufferString(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		logger.Warn("构造 back-channel logout 请求失败", "application", app.Code, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := backchannelHTTPClient.Do(req)
	if err != nil {
		s.audits.Record(ctx, userName, app.InstitutionID, "oauth.backchannel_logout", app.Code,
			map[string]interface{}{"status": "failed", "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	status := "success"
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status = "failed"
	}
	s.audits.Record(ctx, userName, app.InstitutionID, "oauth.backchannel_logout", app.Code,
		map[string]interface{}{"status": status, "http_status": resp.StatusCode})
}
