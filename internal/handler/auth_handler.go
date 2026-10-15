package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *service.AuthService
	userService *service.UserService
	appService  *service.ApplicationService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *service.AuthService, userService *service.UserService, appService *service.ApplicationService) *AuthHandler {
	return &AuthHandler{authService: authService, userService: userService, appService: appService}
}

// publicInstitution 解析公开面的组织引用：
//   - institution_id（主键）：**权威引用**，解析成组织标识后交给服务层（服务层内部以标识定位）；
//   - institution（组织标识）：兼容既有链接与旧客户端。
//
// 为什么要有它：管理面 `?institution=` 是**主键**（见 instID），而公开面过去同一个参数名装的是
// **组织标识**——同名不同义极易踩坑。公开面现统一新增 institution_id 并优先使用；
// 给了主键却解析不到时不回落（否则会静默登进别的组织）。
func (h *AuthHandler) publicInstitution(c *gin.Context) string {
	if v := query(c, "institution_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return ""
		}
		name, err := h.authService.InstitutionNameByID(c.Request.Context(), id)
		if err != nil {
			return ""
		}
		return name
	}
	return query(c, "institution")
}

// Login 密码登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req auth.LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	// 「信任设备」令牌来自 Cookie（不参与请求体绑定）
	if token, err := c.Cookie(auth.MfaTrustCookieName); err == nil {
		req.MfaTrust = token
	}
	resp, sessionID, err := h.authService.Login(c.Request.Context(), req, c.Request.Host, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	// 勾选「记住此设备」并通过二次校验 → 下发信任 Cookie
	if resp.MfaTrustToken != "" && resp.MfaTrustMaxAge > 0 {
		c.SetCookie(auth.MfaTrustCookieName, resp.MfaTrustToken, resp.MfaTrustMaxAge, "/", "", h.authService.CookieSecure(), true)
	}
	if sessionID == "" {
		// 需要 MFA 二次校验
		bizerrors.RenderSuccess(c, resp)
		return
	}
	if err := h.setSessionCookie(c, sessionID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, resp)
}

// Logout 登出
func (h *AuthHandler) Logout(c *gin.Context) {
	if token, err := c.Cookie(auth.TokenCookieName); err == nil && token != "" {
		_ = h.authService.LogoutByToken(c.Request.Context(), token)
	}
	c.SetCookie(auth.TokenCookieName, "", -1, "/", "", h.authService.CookieSecure(), true)
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Status 当前会话状态
func (h *AuthHandler) Status(c *gin.Context) {
	token, _ := c.Cookie(auth.TokenCookieName)
	bizerrors.RenderSuccess(c, h.authService.StatusByToken(c.Request.Context(), token))
}

// Providers 返回可用外部身份提供方（可按组织/应用过滤；或按 client_id 定位应用）
func (h *AuthHandler) Providers(c *gin.Context) {
	var providers []auth.OAuthProviderInfo
	if clientID := query(c, "client_id"); clientID != "" {
		providers = h.authService.ListAppProvidersByClientID(c.Request.Context(), clientID)
	} else {
		providers = h.authService.ListAppProviders(c.Request.Context(), h.publicInstitution(c), query(c, "application"))
	}
	bizerrors.RenderSuccess(c, gin.H{"providers": providers})
}

// AppInfo 返回登录页所需的公开应用信息（策略、字段编排、可用提供方；不含密钥）
// 支持两种定位方式：?client_id=...（应用驱动登录页）或 ?institution=&application=。
func (h *AuthHandler) AppInfo(c *gin.Context) {
	if clientID := query(c, "client_id"); clientID != "" {
		info, err := h.authService.AppInfoByClientID(c.Request.Context(), clientID)
		if err != nil {
			bizerrors.Fail(c, err)
			return
		}
		bizerrors.RenderSuccess(c, info)
		return
	}
	info, err := h.authService.AppInfo(c.Request.Context(), h.publicInstitution(c), query(c, "application"))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, info)
}

// OAuthInfo 返回平台级 OAuth 接入信息（回调地址 / 门户地址），供控制台「开发信息」展示。
func (h *AuthHandler) OAuthInfo(c *gin.Context) {
	callback, homepage := h.authService.OAuthPlatformInfo()
	bizerrors.RenderSuccess(c, gin.H{"callback_url": callback, "homepage_url": homepage})
}

// Institutions 返回登录页可选组织（公开端点，仅名称与显示名）
func (h *AuthHandler) Institutions(c *gin.Context) {
	options, err := h.authService.InstitutionOptions(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"institutions": options})
}

// Captcha 返回组织当前的人机校验挑战（公开）
func (h *AuthHandler) Captcha(c *gin.Context) {
	challenge, err := h.authService.Captcha(c.Request.Context(), h.publicInstitution(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, challenge)
}

// SendCode 发送登录验证码（公开）
func (h *AuthHandler) SendCode(c *gin.Context) {
	var req service.CodeSendRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.authService.SendLoginCode(c.Request.Context(), req, c.Request.Host, c.ClientIP()); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "sent"})
}

// CodeLogin 验证码登录（公开）
func (h *AuthHandler) CodeLogin(c *gin.Context) {
	var req service.CodeLoginRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, sessionID, err := h.authService.CodeLogin(c.Request.Context(), req, c.Request.Host, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if err := h.setSessionCookie(c, sessionID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, resp)
}

// OAuthAuthorize 发起 SSO 授权（返回授权地址）
func (h *AuthHandler) OAuthAuthorize(c *gin.Context) {
	provider := pathParam(c, "provider")
	from := query(c, "from")
	redirect := query(c, "redirect_uri")
	// institution 用于在**组织内**定位提供商（提供商名称只在组织内唯一）
	url, state, err := h.authService.Authorize(c.Request.Context(), h.publicInstitution(c), provider, from, redirect)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"authorization_url": url, "state": state})
}

// OAuthCallback 处理 SSO 回调（建立本地会话或完成身份绑定）
func (h *AuthHandler) OAuthCallback(c *gin.Context) {
	code := query(c, "code")
	state := query(c, "state")
	if code == "" || state == "" {
		bizerrors.Fail(c, bizerrors.ErrSSOStateInvalid)
		return
	}
	resp, sessionID, err := h.authService.Callback(c.Request.Context(), code, state, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if sessionID != "" {
		if err := h.setSessionCookie(c, sessionID); err != nil {
			bizerrors.Fail(c, err)
			return
		}
	}
	bizerrors.RenderSuccess(c, resp)
}

// AuthorizeIdentityLink 发起当前用户的身份绑定流程。
func (h *AuthHandler) AuthorizeIdentityLink(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	url, state, err := h.authService.AuthorizeIdentityLink(
		c.Request.Context(), "", pathParam(c, "provider"), authCtx.SessionID,
	)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"authorization_url": url, "state": state})
}

// Signup 自助注册（托管注册页 /signup 的后端入口，字段由应用 signup_items 编排驱动）
func (h *AuthHandler) Signup(c *gin.Context) {
	var req service.SignupRequest
	if !bindJSON(c, &req) {
		return
	}
	user, session, err := h.authService.Signup(c.Request.Context(), req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	// 应用开启 signup_auto_signin 时直接建立会话，返回与登录一致的响应
	if session != nil {
		if err := h.setSessionCookie(c, session.ID); err != nil {
			bizerrors.Fail(c, err)
			return
		}
		if resp := h.authService.LoginResponse(user, session, req.From); resp != nil {
			bizerrors.RenderSuccess(c, resp, http.StatusCreated)
			return
		}
	}
	bizerrors.RenderSuccess(c, gin.H{"institution": user.InstitutionID, "name": user.Name}, http.StatusCreated)
}

// SendMagicLink 发送邮件魔法链接（公开；防用户枚举，恒返回 sent）
func (h *AuthHandler) SendMagicLink(c *gin.Context) {
	var req service.MagicLinkRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.authService.SendMagicLink(c.Request.Context(), req, c.Request.Host, c.ClientIP()); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "sent"})
}

// VerifyMagicLink 校验魔法链接并建立会话（公开；token 一次性消费）
func (h *AuthHandler) VerifyMagicLink(c *gin.Context) {
	var req service.MagicLinkVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, sessionID, err := h.authService.VerifyMagicLink(c.Request.Context(), req, c.Request.Host, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if err := h.setSessionCookie(c, sessionID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, resp)
}

// MfaSetup 生成 TOTP 密钥
func (h *AuthHandler) MfaSetup(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	result, err := h.userService.SetupMFA(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, result)
}

// MfaVerify 校验动态码并启用 MFA
func (h *AuthHandler) MfaVerify(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}
	codes, err := h.userService.VerifyMFA(c.Request.Context(), authCtx.InstitutionID, authCtx.Name, req.Code)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	// 组织强制 MFA 的「待绑定」会话：绑定完成即解除门禁
	if authCtx.MfaEnrollRequired {
		_ = h.authService.ClearMfaEnroll(c.Request.Context(), authCtx.SessionID)
	}
	bizerrors.RenderSuccess(c, gin.H{"recovery_codes": codes})
}

// MfaDisable 关闭 MFA
func (h *AuthHandler) MfaDisable(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	if err := h.userService.DisableMFA(c.Request.Context(), authCtx.Name, authCtx.InstitutionID, authCtx.Name); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ChangePassword 修改当前用户密码
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	var req auth.ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.userService.ChangePassword(c.Request.Context(), authCtx.InstitutionID, authCtx.Name, req.OldPassword, req.NewPassword); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	c.SetCookie(auth.TokenCookieName, "", -1, "/", "", h.authService.CookieSecure(), true)
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, sessionID string) error {
	jwtToken, err := h.authService.IssueJWT(sessionID)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "签发会话失败", err)
	}
	maxAge := int(h.authService.SessionTTL().Seconds())
	c.SetCookie(auth.TokenCookieName, jwtToken, maxAge, "/", "", h.authService.CookieSecure(), true)
	c.Header("X-Polaris-Session", sessionID)
	return nil
}
