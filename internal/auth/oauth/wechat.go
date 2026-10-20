package oauth

import (
	"context"
	"fmt"
	"net/http"

	"polaris/internal/config"
)

const (
	weChatAuthBase = "https://open.weixin.qq.com/connect/qrconnect"
	weChatTokenURL = "https://api.weixin.qq.com/sns/oauth2/access_token"
	weChatUserURL  = "https://api.weixin.qq.com/sns/userinfo"
)

// WeChatProvider 微信（开放平台扫码登录）连接器。
type WeChatProvider struct {
	name        string
	displayName string
	appID       string
	appSecret   string
	client      *Client
}

// NewWeChatProvider 创建微信连接器
func NewWeChatProvider(name string, cfg config.OAuthProviderConfig, client *Client) *WeChatProvider {
	display := cfg.DisplayName
	if display == "" {
		display = "微信"
	}
	return &WeChatProvider{name: name, displayName: display, appID: cfg.ClientID, appSecret: cfg.ClientSecret, client: client}
}

// Name 连接器名
func (p *WeChatProvider) Name() string { return p.name }

// DisplayName 展示名
func (p *WeChatProvider) DisplayName() string { return p.displayName }

// AuthCodeURL 生成微信扫码授权地址
func (p *WeChatProvider) AuthCodeURL(state, redirectURI string) string {
	return buildQuery(weChatAuthBase, map[string]string{
		"appid":         p.appID,
		"redirect_uri":  redirectURI,
		"response_type": "code",
		"scope":         "snsapi_login",
		"state":         state,
	}) + "#wechat_redirect"
}

// Exchange 用授权码换取微信用户信息
func (p *WeChatProvider) Exchange(ctx context.Context, code, redirectURI string) (*UserInfo, error) {
	tokenURL := buildQuery(weChatTokenURL, map[string]string{
		"appid":      p.appID,
		"secret":     p.appSecret,
		"code":       code,
		"grant_type": "authorization_code",
	})
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return nil, err
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
		UnionID     string `json:"unionid"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := p.client.getJSON(tokenReq, &tokenResp); err != nil {
		return nil, fmt.Errorf("微信换取 token 失败: %w", err)
	}
	if tokenResp.ErrCode != 0 || tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("微信换取 token 失败: %d %s", tokenResp.ErrCode, tokenResp.ErrMsg)
	}

	userURL := buildQuery(weChatUserURL, map[string]string{
		"access_token": tokenResp.AccessToken,
		"openid":       tokenResp.OpenID,
		"lang":         "zh_CN",
	})
	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return nil, err
	}
	var u struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		Nickname   string `json:"nickname"`
		HeadImgURL string `json:"headimgurl"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := p.client.getJSON(userReq, &u); err != nil {
		return nil, fmt.Errorf("微信获取用户信息失败: %w", err)
	}
	if u.ErrCode != 0 {
		return nil, fmt.Errorf("微信获取用户信息失败: %d %s", u.ErrCode, u.ErrMsg)
	}
	id := u.UnionID
	if id == "" {
		id = u.OpenID
	}
	return &UserInfo{
		ProviderUserID: id,
		UnionID:        u.UnionID,
		Username:       u.Nickname,
		DisplayName:    u.Nickname,
		Avatar:         u.HeadImgURL,
	}, nil
}
