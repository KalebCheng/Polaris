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
	providerpkg "polaris/internal/service/provider"
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
	ProviderID  int64  `json:"provider_id,omitempty"`
	TemplateID  int64  `json:"template_id,omitempty"`
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
	app, err := s.resolveMagicLinkApp(ctx, org.ID, req)
	if err != nil {
		return err
	}
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return err
	}
	method, ok, err := s.findSigninMethod(ctx, org.ID, app, repository.SigninMethodMagicLink)
	if err != nil {
		return err
	}
	if !ok || method.ProviderID == 0 {
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
			map[string]interface{}{"delivered": false, "application": applicationCode(app)})
		return nil
	}

	_, tpl, err := s.templates.ResolveNotification(ctx, org.ID, method.ProviderID, method.TemplateID,
		repository.TemplateSceneMagicLink, providerpkg.CategoryEmail)
	if err != nil {
		return err
	}
	ttl := durationFromSeconds(method.TTLSeconds)
	token := randomBase64(32)
	payload, err := json.Marshal(magicLinkPayload{
		Institution: resolvedOrg,
		Application: applicationCode(app),
		Email:       email,
		From:        req.From,
		ProviderID:  method.ProviderID,
		TemplateID:  tpl.ID,
		IssuedAt:    time.Now().Unix(),
	})
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成魔法链接失败", err)
	}
	if err := s.magicLinks.Save(ctx, token, payload, ttl); err != nil {
		return bizerrors.Wrap(bizerrors.CodeRedisError, "redis error", err)
	}

	provider, tpl, err := s.templates.ResolveNotification(ctx, org.ID, method.ProviderID, tpl.ID,
		repository.TemplateSceneMagicLink, providerpkg.CategoryEmail)
	if err != nil {
		return err
	}
	sender, err := notificationSender(provider, tpl)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "notification provider unavailable", err)
	}
	clientID := ""
	if app != nil {
		clientID = app.ClientID
	}
	link := s.magicLinkURL(host, token, clientID)
	minutes := int(ttl.Minutes())
	if minutes <= 0 {
		minutes = 1
	}
	if err := sender.Send(ctx, notify.Message{
		To: email,
		Vars: map[string]string{
			providerpkg.TemplateVarLink:        link,
			providerpkg.TemplateVarMinutes:     strconv.Itoa(minutes),
			providerpkg.TemplateVarInstitution: resolvedOrg,
			providerpkg.TemplateVarApplication: applicationDisplayName(app, org),
		},
	}); err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "send notification failed", err)
	}
	s.audits.Record(ctx, email, org.ID, "auth.magic_link.send", email,
		map[string]interface{}{"delivered": true, "application": applicationCode(app)})
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
	app, err := s.resolveMagicLinkAppByCode(ctx, org.ID, payload.Application)
	if err != nil {
		return nil, "", err
	}
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return nil, "", err
	}
	method, ok, err := s.findSigninMethod(ctx, org.ID, app, repository.SigninMethodMagicLink)
	if err != nil {
		return nil, "", err
	}
	if !ok || method.ProviderID == 0 {
		return nil, "", bizerrors.New(bizerrors.CodeForbidden, "magic link sign-in is disabled for this application")
	}
	if payload.ProviderID > 0 {
		if _, _, resolveErr := s.templates.ResolveNotification(ctx, org.ID, payload.ProviderID,
			payload.TemplateID, repository.TemplateSceneMagicLink, providerpkg.CategoryEmail); resolveErr != nil {
			return nil, "", resolveErr
		}
	}
	user, err := s.users.GetByEmail(ctx, org.ID, payload.Email)
	if err != nil {
		return nil, "", bizerrors.ErrInvalidCredentials
	}
	// 应用准入：魔法链接恒为应用驱动（发链接时已要求该应用启用本方式），按链接记录的应用判定。
	if err := s.enforceAppAccess(ctx, app, user); err != nil {
		return nil, "", err
	}
	s.applyPasswordChangeRequirement(ctx, org.ID, user)
	session, err := s.sessions.Create(ctx, user, applicationCode(app), ip, userAgent)
	if err != nil {
		return nil, "", err
	}
	challenge, _, _, err := s.applyMFA(ctx, session, method, user, "", "", false)
	if err != nil {
		_ = s.sessions.Delete(ctx, session)
		return nil, "", err
	}
	s.enforceSessionPolicy(ctx, app, user, session.ID)
	if !session.MfaPending {
		_ = s.users.UpdateLastSignin(ctx, user.ID, time.Now(), ip)
		s.audits.Record(ctx, user.Name, user.InstitutionID, "auth.magic_link.signin", user.Name,
			map[string]interface{}{"application": applicationCode(app)})
		if s.provisioning != nil {
			s.provisioning.TriggerJIT(ctx, user.InstitutionID, applicationCode(app), user.Name)
		}
	}
	resp := s.loginResponse(user, session, payload.From)
	resp.Mfa = challenge
	return resp, session.ID, nil
}

// resolveMagicLinkApp 解析显式应用上下文；两者都空表示平台级魔法链接。
func (s *AuthService) resolveMagicLinkApp(ctx context.Context, institutionID int64, req MagicLinkRequest) (*repository.Application, error) {
	if clientID := strings.TrimSpace(req.ClientID); clientID != "" {
		app, err := s.apps.GetByClientID(ctx, clientID)
		if err != nil {
			return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("client_id", clientID)
		}
		if app.InstitutionID != institutionID {
			return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("client_id", clientID)
		}
		return app, nil
	}
	if code := strings.TrimSpace(req.Application); code != "" {
		return s.resolveMagicLinkAppByCode(ctx, institutionID, code)
	}
	return nil, nil
}

func (s *AuthService) resolveMagicLinkAppByCode(ctx context.Context, institutionID int64, code string) (*repository.Application, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	app, err := s.apps.GetByCode(ctx, institutionID, code)
	if err != nil {
		return nil, bizerrors.NewWithCode(bizerrors.CodeApplicationNotFound).WithDetail("application", code)
	}
	return app, nil
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

// applicationCode 取应用标识（应用可为 nil：登录链路允许无应用）。
func applicationCode(app *repository.Application) string {
	if app == nil {
		return ""
	}
	return app.Code
}
