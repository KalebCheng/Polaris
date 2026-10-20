package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"polaris/internal/config"
)

const (
	dingTalkAuthBase = "https://login.dingtalk.com/oauth2/auth"
	dingTalkTokenURL = "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"
	dingTalkUserURL  = "https://api.dingtalk.com/v1.0/contact/users/me"
)

// DingTalkProvider 钉钉扫码登录连接器。
type DingTalkProvider struct {
	name         string
	displayName  string
	clientID     string
	clientSecret string
	client       *Client
}

// NewDingTalkProvider 创建钉钉连接器
func NewDingTalkProvider(name string, cfg config.OAuthProviderConfig, client *Client) *DingTalkProvider {
	display := cfg.DisplayName
	if display == "" {
		display = "钉钉"
	}
	return &DingTalkProvider{name: name, displayName: display, clientID: cfg.ClientID, clientSecret: cfg.ClientSecret, client: client}
}

// Name 连接器名
func (p *DingTalkProvider) Name() string { return p.name }

// DisplayName 展示名
func (p *DingTalkProvider) DisplayName() string { return p.displayName }

// AuthCodeURL 生成钉钉授权地址
func (p *DingTalkProvider) AuthCodeURL(state, redirectURI string) string {
	return buildQuery(dingTalkAuthBase, map[string]string{
		"redirect_uri":  redirectURI,
		"response_type": "code",
		"client_id":     p.clientID,
		"scope":         "openid",
		"prompt":        "consent",
		"state":         state,
	})
}

// Exchange 用授权码换取钉钉用户信息
func (p *DingTalkProvider) Exchange(ctx context.Context, code, redirectURI string) (*UserInfo, error) {
	if err := ensureRedirect(redirectURI); err != nil {
		return nil, err
	}
	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     p.clientID,
		"clientSecret": p.clientSecret,
		"code":         code,
		"grantType":    "authorization_code",
	})
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, dingTalkTokenURL, bytes.NewReader(tokenBody))
	if err != nil {
		return nil, err
	}
	var tokenResp struct {
		AccessToken string `json:"accessToken"`
		Message     string `json:"message"`
	}
	if err := p.client.postJSON(tokenReq, &tokenResp); err != nil {
		return nil, fmt.Errorf("钉钉换取 token 失败: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("钉钉未返回 accessToken: %s", tokenResp.Message)
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dingTalkUserURL, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("x-acs-dingtalk-access-token", tokenResp.AccessToken)
	var u struct {
		Nick      string `json:"nick"`
		UnionID   string `json:"unionId"`
		OpenID    string `json:"openId"`
		AvatarURL string `json:"avatarUrl"`
		Mobile    string `json:"mobile"`
		Email     string `json:"email"`
	}
	if err := p.client.getJSON(userReq, &u); err != nil {
		return nil, fmt.Errorf("钉钉获取用户信息失败: %w", err)
	}
	id := u.UnionID
	if id == "" {
		id = u.OpenID
	}
	return &UserInfo{
		ProviderUserID: id,
		UnionID:        u.UnionID,
		Username:       u.Nick,
		DisplayName:    u.Nick,
		Email:          u.Email,
		Phone:          u.Mobile,
		Avatar:         u.AvatarURL,
	}, nil
}
