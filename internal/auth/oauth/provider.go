// Package oauth 实现 Polaris 作为服务提供方（SP）接入外部身份提供方的能力。
//
// 内置连接器按需实现：钉钉（dingtalk）、微信（wechat）、飞书（feishu/lark），
// 以及通用 OIDC。其它第三方提供方通过实现 Provider 接口扩展。
package oauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// UserInfo 外部身份提供方返回的归一化用户信息。
type UserInfo struct {
	ProviderUserID string                 `json:"provider_user_id"`
	UnionID        string                 `json:"union_id,omitempty"`
	Username       string                 `json:"username,omitempty"`
	DisplayName    string                 `json:"display_name,omitempty"`
	Email          string                 `json:"email,omitempty"`
	Phone          string                 `json:"phone,omitempty"`
	Avatar         string                 `json:"avatar,omitempty"`
	Raw            map[string]interface{} `json:"raw,omitempty"`
}

// Provider 外部身份提供方连接器接口。
type Provider interface {
	// Name 连接器名（对应 auth.oauth.providers 的键）
	Name() string
	// DisplayName 展示名
	DisplayName() string
	// AuthCodeURL 生成授权地址
	AuthCodeURL(state, redirectURI string) string
	// Exchange 用授权码换取用户信息
	Exchange(ctx context.Context, code, redirectURI string) (*UserInfo, error)
}

// PKCEProvider 支持授权码 + PKCE（S256）的连接器。
// 调用方按需类型断言：未实现该接口的连接器继续走普通授权码流程。
type PKCEProvider interface {
	// AuthCodeURLWithPKCE 生成带 code_challenge 的授权地址
	AuthCodeURLWithPKCE(state, redirectURI, challenge string) string
	// ExchangeWithPKCE 用授权码与 code_verifier 换取用户信息
	ExchangeWithPKCE(ctx context.Context, code, redirectURI, verifier string) (*UserInfo, error)
}

// Client HTTP 客户端（复用默认 transport，出向自动接入链路追踪）
type Client struct {
	http *http.Client
}

// NewClient 创建带超时的 HTTP 客户端
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{http: &http.Client{Timeout: timeout}}
}

// buildQuery 拼接查询串。
func buildQuery(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	if strings.Contains(base, "?") {
		b.WriteString("&")
	} else {
		b.WriteString("?")
	}
	first := true
	for k, v := range params {
		if !first {
			b.WriteString("&")
		}
		first = false
		b.WriteString(escape(k))
		b.WriteString("=")
		b.WriteString(escape(v))
	}
	return b.String()
}

func escape(s string) string {
	return urlQueryEscape(s)
}

// ensureRedirect 校验 redirect_uri 非空。
func ensureRedirect(redirectURI string) error {
	if strings.TrimSpace(redirectURI) == "" {
		return fmt.Errorf("redirect_uri 不能为空")
	}
	return nil
}
