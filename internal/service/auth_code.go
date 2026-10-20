package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/notify"
	providerpkg "polaris/internal/service/provider"
)

// 邮件 / 短信验证码登录：
//   - 发码：按组织内 category=Email/SMS 的启用提供商发送，受"同目标 60s 一次 + 每小时上限"限流；
//   - 校验：验证码存 Redis，一次性消费；
//   - 登录：命中用户直接建会话；未命中且应用允许自助注册时 JIT 建号。

const (
	codeTTL            = 5 * time.Minute
	codeSendMinGap     = 60 * time.Second
	codeSendHourlyMax  = int64(10)
	verifyCodeKeyScope = "polaris:verify:"
	codeSendRateScope  = "polaris:verify_send:"
)

type codeChannelStrategy struct {
	category     providerpkg.Category
	signinMethod string
	scene        string
	validate     func(string) error
	ttl          func(*repository.Application) time.Duration
	findUser     func(context.Context, *AuthService, int64, string) (*repository.User, error)
	setIdentity  func(*repository.User, string)
}

var codeChannelStrategies = map[CodeKind]codeChannelStrategy{
	CodeKindEmail: {
		category: providerpkg.CategoryEmail, signinMethod: repository.SigninMethodEmailCode,
		scene: repository.TemplateSceneLoginCode,
		validate: func(target string) error {
			if !strings.Contains(target, "@") {
				return bizerrors.New(bizerrors.CodeInvalidParams, "invalid email address")
			}
			return nil
		},
		ttl: func(app *repository.Application) time.Duration { return durationFromSeconds(app.EmailCodeTTLSeconds) },
		findUser: func(ctx context.Context, s *AuthService, institutionID int64, target string) (*repository.User, error) {
			return s.users.GetByEmail(ctx, institutionID, target)
		},
		setIdentity: func(user *repository.User, target string) {
			user.Email = target
			user.EmailVerified = true
		},
	},
	CodeKindSMS: {
		category: providerpkg.CategorySMS, signinMethod: repository.SigninMethodSMSCode,
		scene:    repository.TemplateSceneLoginCode,
		validate: func(string) error { return nil },
		ttl:      func(app *repository.Application) time.Duration { return durationFromSeconds(app.SMSCodeTTLSeconds) },
		findUser: func(ctx context.Context, s *AuthService, institutionID int64, target string) (*repository.User, error) {
			return s.findUserByPhone(ctx, institutionID, target)
		},
		setIdentity: func(user *repository.User, target string) {
			user.Phone = target
			user.PhoneVerified = true
		},
	},
}

// CodeKind 验证码通道
type CodeKind string

// 支持的通道
const (
	CodeKindEmail CodeKind = "email"
	CodeKindSMS   CodeKind = "sms"
)

// CodeSendRequest 发送验证码请求
type CodeSendRequest struct {
	// Institution 组织标识（兼容）；InstitutionID 组织主键（权威）
	Institution   string `json:"institution"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application"`
	// ClientID 应用驱动入口的应用上下文（优先于 Application；两者都空 = 平台登录，不判定准入）
	ClientID     string `json:"client_id"`
	Kind         string `json:"kind" binding:"required"`
	Target       string `json:"target" binding:"required"`
	CaptchaID    string `json:"captcha_id"`
	CaptchaCode  string `json:"captcha_code"`
	CaptchaToken string `json:"captcha_token"`
}

// CodeLoginRequest 验证码登录请求
type CodeLoginRequest struct {
	Institution   string `json:"institution"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application"`
	// ClientID 应用驱动入口的应用上下文（优先于 Application；两者都空 = 平台登录，不判定准入）
	ClientID string `json:"client_id"`
	Kind     string `json:"kind" binding:"required"`
	Target   string `json:"target" binding:"required"`
	Code     string `json:"code" binding:"required"`
	From     string `json:"from"`
}

// Captcha 返回组织当前的人机校验挑战
func (s *AuthService) Captcha(ctx context.Context, institution string) (*CaptchaChallenge, error) {
	if s.captcha == nil {
		return &CaptchaChallenge{Required: false}, nil
	}
	return s.captcha.Challenge(ctx, institution)
}

// SendLoginCode 发送登录验证码
func (s *AuthService) SendLoginCode(ctx context.Context, req CodeSendRequest, host, remoteIP string) error {
	resolvedOrg, err := s.resolveLoginInstitution(ctx, req.InstitutionID, req.Institution, host, strings.TrimSpace(req.Target))
	if err != nil {
		return err
	}
	req.Institution = resolvedOrg
	app, err := s.resolveApplication(ctx, req.Institution, req.Application)
	if err != nil {
		return err
	}
	if err := s.enforceSigninPolicy(app, remoteIP); err != nil {
		return err
	}
	kind, err := normalizeCodeKind(req.Kind)
	if err != nil {
		return err
	}
	channel := codeChannelStrategies[kind]
	// 配置即生效：该类型验证码登录必须是应用已启用的登录方式
	if !app.HasSigninMethod(channel.signinMethod) {
		return bizerrors.New(bizerrors.CodeForbidden, "code sign-in is disabled for this application")
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return bizerrors.New(bizerrors.CodeInvalidParams, "target is required")
	}
	if err := channel.validate(target); err != nil {
		return err
	}

	// 人机校验（组织启用验证码提供商时强制）
	if s.captcha != nil {
		if err := s.captcha.Verify(ctx, req.Institution, req.CaptchaID, req.CaptchaCode, req.CaptchaToken, remoteIP); err != nil {
			return err
		}
	}
	// 发送频率限制：同目标 60 秒一次
	if s.rateLimiter != nil {
		key := codeSendRateScope + string(kind) + ":" + req.Institution + ":" + target
		if n, _ := s.rateLimiter.Get(ctx, key); n > 0 {
			return bizerrors.ErrVerifyCodeTooMany.WithDetail("retry_after_seconds", int(codeSendMinGap.Seconds()))
		}
		if _, incrErr := s.rateLimiter.Incr(ctx, key, codeSendMinGap); incrErr != nil {
			return incrErr
		}
		hourKey := key + ":hour"
		if n, _ := s.rateLimiter.Get(ctx, hourKey); n >= codeSendHourlyMax {
			return bizerrors.ErrVerifyCodeTooMany.WithDetail("window", "hourly")
		}
		if _, incrErr := s.rateLimiter.Incr(ctx, hourKey, time.Hour); incrErr != nil {
			return incrErr
		}
	}

	method, ok := app.FindSigninMethod(channel.signinMethod)
	if !ok || method.ProviderID == 0 {
		return bizerrors.ErrProviderUnavailable.WithDetail("provider", "login_code")
	}
	provider, tpl, err := s.templates.ResolveNotification(ctx, app.InstitutionID, method.ProviderID, method.TemplateID,
		channel.scene, channel.category)
	if err != nil {
		return err
	}
	sender, err := notificationSender(provider, tpl)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "notification provider unavailable", err)
	}

	code, err := generateNumericCode(6)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成验证码失败", err)
	}
	if s.codes == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("reason", "code store not configured")
	}
	if err := s.codes.Save(ctx, verifyCodeKey(req.Institution, kind, provider.ID, target), code, channel.ttl(app)); err != nil {
		return err
	}
	// 模板变量的取值来源：验证码 / 有效期 / 接收号码 / 组织与应用名（阿里云只装配模板声明过的那些）
	ttlMinutes := strconv.Itoa(int(channel.ttl(app).Minutes()))
	if ttlMinutes == "0" {
		ttlMinutes = "1"
	}
	msg := notify.Message{
		To: target, Code: code,
		Vars: map[string]string{
			providerpkg.TemplateVarCode:        code,
			providerpkg.TemplateVarMinutes:     ttlMinutes,
			providerpkg.TemplateVarPhone:       target,
			providerpkg.TemplateVarInstitution: req.Institution,
			providerpkg.TemplateVarApplication: orDefault(app.Name, app.Code),
		},
	}
	if err := sender.Send(ctx, msg); err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "send notification failed", err)
	}
	s.audits.Record(ctx, target, app.InstitutionID, "auth.send_code", target, map[string]interface{}{"kind": kind})
	return nil
}

// CodeLogin 校验验证码并登录
func (s *AuthService) CodeLogin(ctx context.Context, req CodeLoginRequest, host, ip, userAgent string) (*auth.LoginResponse, string, error) {
	kind, err := normalizeCodeKind(req.Kind)
	if err != nil {
		return nil, "", err
	}
	channel := codeChannelStrategies[kind]
	target := strings.TrimSpace(req.Target)
	resolvedOrg, err := s.resolveLoginInstitution(ctx, req.InstitutionID, req.Institution, host, target)
	if err != nil {
		return nil, "", err
	}
	req.Institution = resolvedOrg
	// 应用上下文：显式携带时以它为准（准入判定据此生效）；未携带时按既有语义回退默认应用，
	// 但**不做准入判定**（Spec FR-3 作用域：平台登录不判定）。
	appCtx, err := s.resolveAppContext(ctx, req.InstitutionID, req.Institution, req.Application, req.ClientID)
	if err != nil {
		return nil, "", err
	}
	app := appCtx
	if app == nil {
		app, err = s.resolveApplication(ctx, req.Institution, "")
		if err != nil {
			return nil, "", err
		}
	}
	if err := s.enforceSigninPolicy(app, ip); err != nil {
		return nil, "", err
	}
	methodName := channel.signinMethod
	method, ok := app.FindSigninMethod(methodName)
	if !ok || method.ProviderID == 0 {
		return nil, "", bizerrors.New(bizerrors.CodeForbidden, "code sign-in is disabled for this application")
	}
	provider, _, err := s.templates.ResolveNotification(ctx, app.InstitutionID, method.ProviderID, method.TemplateID,
		channel.scene, channel.category)
	if err != nil {
		return nil, "", err
	}
	if s.codes == nil {
		return nil, "", bizerrors.ErrProviderUnavailable.WithDetail("reason", "code store not configured")
	}
	stored, err := s.codes.Take(ctx, verifyCodeKey(req.Institution, kind, provider.ID, target))
	if err != nil {
		return nil, "", err
	}
	if stored == "" || stored != strings.TrimSpace(req.Code) {
		return nil, "", bizerrors.ErrVerifyCodeInvalid
	}

	user, err := s.findUserByCodeTarget(ctx, app, kind, target)
	if err != nil {
		return nil, "", err
	}
	// 应用准入：验证码校验通过、建会话之前（拒绝时不留下任何登录态）
	if err := s.enforceAppAccess(ctx, appCtx, user); err != nil {
		return nil, "", err
	}
	session, err := s.sessions.Create(ctx, user, app.Code, ip, userAgent)
	if err != nil {
		return nil, "", err
	}
	challenge, _, _, err := s.applyMFA(ctx, session, app, signinMethodForCodeKind(kind), user, "", "", false)
	if err != nil {
		_ = s.sessions.Delete(ctx, session)
		return nil, "", err
	}
	s.enforceSessionPolicy(ctx, app, user, session.ID)
	if !session.MfaPending {
		_ = s.users.UpdateLastSignin(ctx, user.ID, time.Now(), ip)
		_ = s.users.SetOnline(ctx, user.ID, true)
		s.audits.Record(ctx, user.Name, user.InstitutionID, "auth.code_login", user.Name, map[string]interface{}{"kind": kind})
		// 登录成功：触发 JIT 建号（与密码登录一致）
		if s.provisioning != nil {
			s.provisioning.TriggerJIT(ctx, user.InstitutionID, app.Code, user.Name)
		}
	}

	from := req.From
	resp := s.loginResponse(user, session, from)
	resp.Mfa = challenge
	return resp, session.ID, nil
}

// findUserByCodeTarget 按邮箱/手机号定位用户，必要时 JIT 建号。
func (s *AuthService) findUserByCodeTarget(ctx context.Context, app *repository.Application, kind CodeKind, target string) (*repository.User, error) {
	channel := codeChannelStrategies[kind]
	if user, err := channel.findUser(ctx, s, app.InstitutionID, target); err == nil {
		return user, nil
	}

	if !app.SelfSignupEnabled {
		return nil, bizerrors.ErrIdentityNotBound.WithDetail("target", target)
	}
	name := s.derivedUserName(ctx, app.InstitutionID, target)
	user := &repository.User{
		UID:            uuid.NewString(),
		InstitutionID:  app.InstitutionID,
		Name:           name,
		Type:           "normal",
		RegisterType:   "code",
		RegisterSource: string(kind),
	}
	channel.setIdentity(user, target)
	if app.DefaultJobTitleID != nil {
		user.SignupApplication = app.Code
	}
	if _, err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	s.addDefaultJobTitle(ctx, app.InstitutionID, *app.DefaultJobTitleID, user.Name)
	s.audits.Record(ctx, name, app.InstitutionID, "user.provision_code", name, map[string]interface{}{"kind": kind})
	// 新建用户：触发"创建即建号"的账号策略（与管理员建号一致）
	if s.provisioning != nil {
		s.provisioning.TriggerUserProvisioning(ctx, user.InstitutionID, user.Name)
	}
	return user, nil
}

// findUserByPhone 按手机号查找用户（同组织内）
func (s *AuthService) findUserByPhone(ctx context.Context, institutionID int64, phone string) (*repository.User, error) {
	users, err := s.users.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "phone", Value: phone, Limit: 1,
	})
	if err != nil || len(users) == 0 {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	return users[0], nil
}

func normalizeCodeKind(kind string) (CodeKind, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "email", "mail":
		return CodeKindEmail, nil
	case "sms", "phone", "mobile":
		return CodeKindSMS, nil
	default:
		return "", bizerrors.New(bizerrors.CodeInvalidParams, "kind must be email or sms").WithDetail("kind", kind)
	}
}

// signinMethodForCodeKind 把验证码类型映射到登录方式名（"配置即生效"的判定入口）。
func signinMethodForCodeKind(kind CodeKind) string {
	return codeChannelStrategies[kind].signinMethod
}

func durationFromSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return codeTTL
	}
	return time.Duration(seconds) * time.Second
}

func verifyCodeKey(institution string, kind CodeKind, providerID int64, target string) string {
	return verifyCodeKeyScope + string(kind) + ":" + institution + ":" + strconv.FormatInt(providerID, 10) + ":" + strings.ToLower(target)
}

// generateNumericCode 生成 n 位数字验证码（密码学随机）
func generateNumericCode(n int) (string, error) {
	var builder strings.Builder
	max := big.NewInt(10)
	for i := 0; i < n; i++ {
		digit, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("生成随机数失败: %w", err)
		}
		builder.WriteString(digit.String())
	}
	return builder.String(), nil
}

// derivedUserName 由邮箱/手机号派生组织内唯一用户名
func (s *AuthService) derivedUserName(ctx context.Context, institutionID int64, target string) string {
	base := target
	if at := strings.Index(target, "@"); at > 0 {
		base = target[:at]
	}
	base = sanitizeName(base)
	if base == "" {
		base = "user-" + randomHex(4)
	}
	name := base
	for i := 0; i < 50; i++ {
		if _, err := s.users.GetByName(ctx, institutionID, name); err != nil {
			return name
		}
		name = base + "-" + randomHex(2)
	}
	return base + "-" + randomHex(4)
}
