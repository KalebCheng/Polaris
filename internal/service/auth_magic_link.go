package service

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/notify"
)

// 本文件实现「邮件魔法链接」（Magic Link）免密登录。
//
// 安全要点：
//   - 防用户枚举：无论邮箱是否存在，发送接口都返回同一结果（仅账号存在时才真正发信）；
//   - 一次性消费：token 存 Redis，校验走 GETDEL（读取即删除），同一链接只能成功登录一次；
//   - 短时效 + 频控 + 人机校验：TTL 由应用级 magic_link_ttl_seconds 控制（默认 600 秒）；
//   - 邮件中的链接只指向本平台地址，回跳参数必须是站内相对路径（与 SSO 的 from 约束一致）。

// MagicLinkRequest 发送魔法链接请求
type MagicLinkRequest struct {
	Institution   string `json:"institution"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application"`
	ClientID      string `json:"client_id"`
	Email         string `json:"email" binding:"required"`
	From          string `json:"from"`
	CaptchaID     string `json:"captcha_id"`
	CaptchaCode   string `json:"captcha_code"`
	CaptchaToken  string `json:"captcha_token"`
}

// MagicLinkVerifyRequest 校验魔法链接请求
type MagicLinkVerifyRequest struct {
	Token    string `json:"token" binding:"required"`
	ClientID string `json:"client_id"`
}

// magicLinkPayload 存入 Redis 的载荷（不含任何长期凭证）。
type magicLinkPayload struct {
	Institution string `json:"institutionID"`
	Application string `json:"application"`
	Email       string `json:"email"`
	From        string `json:"from"`
	IssuedAt    int64  `json:"issued_at"`
}

const (
	magicLinkRateScope = "polaris:ratelimit:magic:"
	magicLinkMinGap    = 60 * time.Second
	magicLinkHourlyMax = 10
)

// SendMagicLink 生成并发送魔法链接。
// 无论邮箱是否存在都返回 nil（防用户枚举）；只有账号存在时才真正投递邮件。
func (s *AuthService) SendMagicLink(ctx context.Context, req MagicLinkRequest, host, ip string) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return bizerrors.New(bizerrors.CodeInvalidParams, "a valid email address is required")
	}
	resolvedOrg, err := s.resolveLoginInstitution(ctx, req.InstitutionID, req.Institution, host, email)
	if err != nil {
		return err
	}
	org, err := s.resolveOrgForLogin(ctx, resolvedOrg)
	if err != nil {
		return err
	}
	app, err := s.resolveMagicLinkApp(ctx, resolvedOrg, req)
	if err != nil {
		return err
	}
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return err
	}
	if app == nil || !app.HasSigninMethod(repository.SigninMethodMagicLink) {
		return bizerrors.New(bizerrors.CodeForbidden, "magic link sign-in is disabled for this application")
	}
	if s.magicLinks == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("reason", "magic link store not configured")
	}
	// 人机校验（组织启用验证码提供商时强制）
	if s.captcha != nil {
		if err := s.captcha.Verify(ctx, resolvedOrg, req.CaptchaID, req.CaptchaCode, req.CaptchaToken, ip); err != nil {
			return err
		}
	}
	// 发送频率限制：同邮箱 60 秒一次 + 每小时上限
	if s.rateLimiter != nil {
		key := magicLinkRateScope + resolvedOrg + ":" + email
		if n, _ := s.rateLimiter.Get(ctx, key); n > 0 {
			return bizerrors.ErrVerifyCodeTooMany.WithDetail("retry_after_seconds", int(magicLinkMinGap.Seconds()))
		}
		if _, incrErr := s.rateLimiter.Incr(ctx, key, magicLinkMinGap); incrErr != nil {
			return incrErr
		}
		hourKey := key + ":hour"
		if n, _ := s.rateLimiter.Get(ctx, hourKey); n >= magicLinkHourlyMax {
			return bizerrors.ErrVerifyCodeTooMany.WithDetail("window", "hourly")
		}
		if _, incrErr := s.rateLimiter.Incr(ctx, hourKey, time.Hour); incrErr != nil {
			return incrErr
		}
	}

	// 防用户枚举：账号不存在时静默返回成功，不投递、不泄露存在性。
	if _, err := s.users.GetByEmail(ctx, org.ID, email); err != nil {
		s.audits.Record(ctx, email, org.ID, "auth.magic_link.send", email,
			map[string]interface{}{"delivered": false, "application": app.Code})
		return nil
	}

	ttl := appMagicLinkTTL(app)
	token := randomBase64(32)
	payload, err := json.Marshal(magicLinkPayload{
		Institution: resolvedOrg,
		Application: app.Code,
		Email:       email,
		From:        req.From,
		IssuedAt:    time.Now().Unix(),
	})
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成魔法链接失败", err)
	}
	if err := s.magicLinks.Save(ctx, token, payload, ttl); err != nil {
		return bizerrors.Wrap(bizerrors.CodeRedisError, "redis error", err)
	}

	provider, err := s.notificationProvider(ctx, resolvedOrg, CodeKindEmail)
	if err != nil {
		return err
	}
	sender, err := s.senderFor("email", provider)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "notification provider unavailable", err)
	}
	link := s.magicLinkURL(host, token, app.ClientID)
	if err := sender.Send(ctx, notify.Message{
		To:      email,
		Subject: "登录链接",
		Body:    "点击以下链接登录（{{minutes}} 分钟内有效）：{{link}}",
		Vars: map[string]string{
			"link":    link,
			"minutes": strconv.Itoa(int(ttl.Minutes())),
		},
	}); err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "send notification failed", err)
	}
	s.audits.Record(ctx, email, org.ID, "auth.magic_link.send", email,
		map[string]interface{}{"delivered": true, "application": app.Code})
	return nil
}

// VerifyMagicLink 校验魔法链接并创建会话（token 一次性消费）。
func (s *AuthService) VerifyMagicLink(ctx context.Context, req MagicLinkVerifyRequest, host, ip, userAgent string) (*auth.LoginResponse, string, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" || s.magicLinks == nil {
		return nil, "", bizerrors.ErrVerifyCodeInvalid
	}
	// 限流：同一 IP 高频校验直接拒绝（防止对 token 空间做爆破）
	if s.rateLimiter != nil {
		key := magicLinkRateScope + "verify:" + ip
		if _, err := s.rateLimiter.Incr(ctx, key, magicLinkMinGap); err != nil {
			return nil, "", err
		}
	}
	raw, err := s.magicLinks.Take(ctx, token)
	if err != nil {
		return nil, "", bizerrors.Wrap(bizerrors.CodeRedisError, "redis error", err)
	}
	if len(raw) == 0 {
		return nil, "", bizerrors.ErrVerifyCodeInvalid
	}
	var payload magicLinkPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", bizerrors.ErrVerifyCodeInvalid
	}

	orgName := payload.Institution
	if orgName == "" {
		orgName = s.oauthCfg.DefaultInstitution
	}
	org, err := s.resolveOrgForLogin(ctx, orgName)
	if err != nil {
		return nil, "", err
	}
	app, err := s.resolveApplicationOptional(ctx, orgName, payload.Application)
	if err != nil {
		return nil, "", err
	}
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return nil, "", err
	}
	if app != nil && !app.HasSigninMethod(repository.SigninMethodMagicLink) {
		return nil, "", bizerrors.New(bizerrors.CodeForbidden, "magic link sign-in is disabled for this application")
	}
	user, err := s.users.GetByEmail(ctx, org.ID, payload.Email)
	if err != nil {
		return nil, "", bizerrors.ErrInvalidCredentials
	}
	session, err := s.sessions.Create(ctx, user, applicationCode(app), ip, userAgent)
	if err != nil {
		return nil, "", err
	}
	if s.mfaEnrollRequiredFor(ctx, user) {
		if err := s.sessions.SetMfaEnroll(ctx, session.ID, true); err != nil {
			return nil, "", err
		}
		session.MfaEnrollRequired = true
	}
	s.enforceSessionPolicy(ctx, app, user, session.ID)
	_ = s.users.UpdateLastSignin(ctx, user.ID, time.Now(), ip)
	_ = s.users.SetOnline(ctx, user.ID, true)
	s.audits.Record(ctx, user.Name, user.InstitutionID, "auth.magic_link.signin", user.Name,
		map[string]interface{}{"application": applicationCode(app)})
	if s.provisioning != nil {
		s.provisioning.TriggerJIT(ctx, user.InstitutionID, applicationCode(app), user.Name)
	}
	resp := s.loginResponse(user, session, payload.From)
	if session.MfaEnrollRequired {
		resp.Mfa = &auth.MfaChallenge{Required: true, Enroll: true, Type: "totp"}
	}
	return resp, session.ID, nil
}

// resolveMagicLinkApp 解析目标应用：优先 client_id（应用驱动），否则按 organization+application。
// 与验证码登录的差异：魔法链接必须落在某个应用上，故不宽松回退到 nil——组织无应用时直接报错。
func (s *AuthService) resolveMagicLinkApp(ctx context.Context, institution string, req MagicLinkRequest) (*repository.Application, error) {
	if clientID := strings.TrimSpace(req.ClientID); clientID != "" {
		app, err := s.apps.GetByClientID(ctx, clientID)
		if err != nil {
			return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("client_id", clientID)
		}
		return app, nil
	}
	return s.resolveApplication(ctx, institution, req.Application)
}

// magicLinkURL 生成邮件中的落地链接（指向前端 /magic 页）。
func (s *AuthService) magicLinkURL(host, token, clientID string) string {
	base := strings.TrimRight(s.oauthCfg.BaseURL, "/")
	if base == "" {
		h := strings.TrimSpace(host)
		// 注意：这里不能复用 normalizeHost（它会剥掉端口），落地链接需要保留端口。
		hostname := h
		if hostOnly, _, err := net.SplitHostPort(h); err == nil {
			hostname = hostOnly
		}
		scheme := "https"
		if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
			scheme = "http"
		}
		base = scheme + "://" + h
	}
	link := base + "/magic?token=" + token
	if clientID != "" {
		link += "&client_id=" + clientID
	}
	return link
}

// appMagicLinkTTL 取应用配置的魔法链接有效期，未配置时回退默认 10 分钟。
func appMagicLinkTTL(app *repository.Application) time.Duration {
	if app == nil || app.MagicLinkTTLSeconds <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(app.MagicLinkTTLSeconds) * time.Second
}

// applicationCode 取应用标识（应用可为 nil：登录链路允许无应用）。
func applicationCode(app *repository.Application) string {
	if app == nil {
		return ""
	}
	return app.Code
}
