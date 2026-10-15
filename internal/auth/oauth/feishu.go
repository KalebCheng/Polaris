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
	feishuAuthBase = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
	feishuTokenURL = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
	feishuUserURL  = "https://open.feishu.cn/open-apis/authen/v1/user_info"
)

// FeishuProvider 飞书（Lark）连接器。
type FeishuProvider struct {
	name        string
	displayName string
	appID       string
	appSecret   string
	scopes      []string
	client      *Client
}

// NewFeishuProvider 创建飞书连接器
func NewFeishuProvider(name string, cfg config.OAuthProviderConfig, client *Client) *FeishuProvider {
	display := cfg.DisplayName
	if display == "" {
		display = "飞书"
	}
	return &FeishuProvider{name: name, displayName: display, appID: cfg.ClientID, appSecret: cfg.ClientSecret, scopes: cfg.Scopes, client: client}
}

// Name 连接器名
func (p *FeishuProvider) Name() string { return p.name }

// DisplayName 展示名
func (p *FeishuProvider) DisplayName() string { return p.displayName }

// AuthCodeURL 生成飞书授权地址
func (p *FeishuProvider) AuthCodeURL(state, redirectURI string) string {
	scope := "contact:user.base:readonly"
	if len(p.scopes) > 0 {
		scope = joinScopes(p.scopes)
	}
	return buildQuery(feishuAuthBase, map[string]string{
		"client_id":     p.appID,
		"redirect_uri":  redirectURI,
		"response_type": "code",
		"scope":         scope,
		"state":         state,
	})
}

// Exchange 用授权码换取飞书用户信息
func (p *FeishuProvider) Exchange(ctx context.Context, code, redirectURI string) (*UserInfo, error) {
	if err := ensureRedirect(redirectURI); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     p.appID,
		"client_secret": p.appSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	})
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Code        int    `json:"code"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := p.client.postJSON(tokenReq, &tokenResp); err != nil {
		return nil, fmt.Errorf("飞书换取 token 失败: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("飞书换取 token 失败: %s %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, feishuUserURL, nil)
	if err != nil {
		return nil, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	var u struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID    string `json:"open_id"`
			UnionID   string `json:"union_id"`
			UserID    string `json:"user_id"`
			Name      string `json:"name"`
			EnName    string `json:"en_name"`
			AvatarURL string `json:"avatar_url"`
			Email     string `json:"email"`
			Mobile    string `json:"mobile"`
		} `json:"data"`
	}
	if err := p.client.getJSON(userReq, &u); err != nil {
		return nil, fmt.Errorf("飞书获取用户信息失败: %w", err)
	}
	if u.Code != 0 {
		return nil, fmt.Errorf("飞书获取用户信息失败: %d %s", u.Code, u.Msg)
	}
	id := u.Data.UnionID
	if id == "" {
		id = u.Data.OpenID
	}
	name := u.Data.Name
	if name == "" {
		name = u.Data.EnName
	}
	return &UserInfo{
		ProviderUserID: id,
		UnionID:        u.Data.UnionID,
		Username:       name,
		DisplayName:    name,
		Email:          u.Data.Email,
		Phone:          u.Data.Mobile,
		Avatar:         u.Data.AvatarURL,
	}, nil
}

func joinScopes(scopes []string) string {
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
