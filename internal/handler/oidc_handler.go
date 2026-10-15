package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// OIDCHandler 作为 OIDC Provider 的端点处理器。
type OIDCHandler struct {
	oidc    *service.OIDCService
	authSvc *service.AuthService
}

// NewOIDCHandler 创建 OIDC 处理器
func NewOIDCHandler(oidc *service.OIDCService, authSvc *service.AuthService) *OIDCHandler {
	return &OIDCHandler{oidc: oidc, authSvc: authSvc}
}

// Discovery 返回 OIDC discovery 文档
func (h *OIDCHandler) Discovery(c *gin.Context) {
	c.JSON(200, h.oidc.Discovery())
}

// JWKS 返回 JWKS
func (h *OIDCHandler) JWKS(c *gin.Context) {
	c.JSON(200, h.oidc.JWKS())
}

// Authorize 授权端点。未登录时重定向到登录页并保留 OAuth 参数，登录后由前端续走本端点
// （对齐 Casdoor 的 /login/oauth/authorize：登录页由应用驱动、登录后回跳 redirect_uri）。
func (h *OIDCHandler) Authorize(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		loginURL := "/login"
		if c.Request.URL.RawQuery != "" {
			loginURL += "?" + c.Request.URL.RawQuery
		}
		c.Redirect(http.StatusFound, loginURL)
		return
	}
	redirect, err := h.oidc.Authorize(c.Request.Context(), service.AuthorizeParams{
		ClientID:            query(c, "client_id"),
		RedirectURI:         query(c, "redirect_uri"),
		ResponseType:        query(c, "response_type"),
		Scope:               query(c, "scope"),
		State:               query(c, "state"),
		Nonce:               query(c, "nonce"),
		CodeChallenge:       query(c, "code_challenge"),
		CodeChallengeMethod: query(c, "code_challenge_method"),
		Institution:         strconv.FormatInt(authCtx.InstitutionID, 10),
		UserName:            authCtx.Name,
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	c.Redirect(http.StatusFound, redirect)
}

// Token 令牌端点（支持 authorization_code / refresh_token / password / client_credentials）。
func (h *OIDCHandler) Token(c *gin.Context) {
	clientID, clientSecret := clientCredentials(c)
	resp, err := h.oidc.Token(c.Request.Context(), service.TokenRequest{
		GrantType:    c.PostForm("grant_type"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Code:         c.PostForm("code"),
		RedirectURI:  c.PostForm("redirect_uri"),
		CodeVerifier: c.PostForm("code_verifier"),
		RefreshToken: c.PostForm("refresh_token"),
		Username:     c.PostForm("username"),
		Password:     c.PostForm("password"),
		Scope:        c.PostForm("scope"),
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	c.JSON(200, resp)
}

// UserInfo 用户信息端点
func (h *OIDCHandler) UserInfo(c *gin.Context) {
	token := bearerToken(c)
	if token == "" {
		bizerrors.Fail(c, bizerrors.ErrInvalidToken)
		return
	}
	info, err := h.oidc.UserInfo(c.Request.Context(), token)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	c.JSON(200, info)
}

// Introspect 令牌内省端点
func (h *OIDCHandler) Introspect(c *gin.Context) {
	token := c.PostForm("token")
	if token == "" {
		token = bearerToken(c)
	}
	c.JSON(200, h.oidc.Introspect(c.Request.Context(), token))
}

// EndSession 结束会话端点
func (h *OIDCHandler) EndSession(c *gin.Context) {
	if token, err := c.Cookie(auth.TokenCookieName); err == nil && token != "" {
		_ = h.authSvc.LogoutByToken(c.Request.Context(), token)
	}
	c.SetCookie(auth.TokenCookieName, "", -1, "/", "", h.authSvc.CookieSecure(), true)
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// clientCredentials 解析客户端凭证（支持 HTTP Basic 与表单）。
func clientCredentials(c *gin.Context) (string, string) {
	if id, secret, ok := c.Request.BasicAuth(); ok {
		return id, secret
	}
	return c.PostForm("client_id"), c.PostForm("client_secret")
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		if t := c.PostForm("access_token"); t != "" {
			return t
		}
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
