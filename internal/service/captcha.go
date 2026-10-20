package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mojocn/base64Captcha"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// 人机校验（Captcha）提供商：
//   - 本地图形验证码（sub_type=image）：服务端生成图片，答案存 Redis（一次性消费，多实例一致）；
//   - 远程校验（hcaptcha / recaptcha / turnstile）：转发 token 到厂商接口核验。
//
// 启用方式：在组织内创建 category=Captcha 且 state=active 的提供商；登录页通过
// /api/v1/auth/app-info 得知是否需要校验，并在登录/发码时携带 captcha_id + captcha_code 或 captcha_token。

// CaptchaService 人机校验服务
type CaptchaService struct {
	runtime *providerpkg.Runtime
	orgs    repository.InstitutionRepository
	captcha *base64Captcha.Captcha
	client  *http.Client
}

// NewCaptchaService 创建人机校验服务（store 建议使用 Redis 实现以支持多实例）
func NewCaptchaService(runtime *providerpkg.Runtime, orgs repository.InstitutionRepository, store base64Captcha.Store) *CaptchaService {
	driver := base64Captcha.NewDriverDigit(52, 160, 5, 0.7, 80)
	return &CaptchaService{
		runtime: runtime,
		orgs:    orgs,
		captcha: base64Captcha.NewCaptcha(driver, store),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// CaptchaChallenge 下发给登录页的挑战
type CaptchaChallenge struct {
	Required  bool   `json:"required"`
	Type      string `json:"type,omitempty"` // image | hcaptcha | recaptcha | turnstile
	SiteKey   string `json:"site_key,omitempty"`
	CaptchaID string `json:"captcha_id,omitempty"`
	Image     string `json:"image,omitempty"` // data URL（仅本地图形验证码）
}

// ActiveProvider 返回组织内启用的验证码提供商（不存在返回 nil）
func (s *CaptchaService) ActiveProvider(ctx context.Context, institution string) *repository.Provider {
	if s.runtime == nil {
		return nil
	}
	org, orgErr := s.orgs.GetByName(ctx, institution)
	if orgErr != nil {
		return nil
	}
	item, err := s.runtime.Active(ctx, org.ID, providerpkg.CategoryCaptcha)
	if err != nil {
		return nil
	}
	return item
}

// kindOf 归一化验证码类型（注册表声明见 provider.Catalog 的 captcha 类别）
func kindOf(item *repository.Provider) string {
	return providerpkg.Kind(item)
}

// Challenge 生成或描述一次人机校验挑战
func (s *CaptchaService) Challenge(ctx context.Context, institution string) (*CaptchaChallenge, error) {
	provider := s.ActiveProvider(ctx, institution)
	if provider == nil {
		return &CaptchaChallenge{Required: false}, nil
	}
	kind := kindOf(provider)
	challenge := &CaptchaChallenge{Required: true, Type: kind, SiteKey: provider.ClientID}
	if kind != "image" {
		return challenge, nil
	}
	id, image, _, err := s.captcha.Generate()
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "failed to generate captcha", err)
	}
	challenge.CaptchaID = id
	challenge.Image = image
	return challenge, nil
}

// Verify 校验人机挑战；组织内无启用的验证码提供商时直接放行。
func (s *CaptchaService) Verify(ctx context.Context, institution string, captchaID, code, remoteToken, remoteIP string) error {
	provider := s.ActiveProvider(ctx, institution)
	if provider == nil {
		return nil
	}
	if kindOf(provider) == "image" {
		if captchaID == "" || code == "" {
			return bizerrors.ErrCaptchaRequired
		}
		if !s.captcha.Verify(captchaID, code, true) {
			return bizerrors.ErrCaptchaInvalid
		}
		return nil
	}
	if remoteToken == "" {
		return bizerrors.ErrCaptchaRequired
	}
	return s.verifyRemote(ctx, provider, kindOf(provider), remoteToken, remoteIP)
}

// verifyRemote 转发 token 到厂商校验接口
func (s *CaptchaService) verifyRemote(ctx context.Context, provider *repository.Provider, kind, token, remoteIP string) error {
	var endpoint string
	switch kind {
	case "hcaptcha":
		endpoint = "https://api.hcaptcha.com/siteverify"
	case "recaptcha":
		endpoint = "https://www.google.com/recaptcha/api/siteverify"
	case "turnstile":
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	default:
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported captcha type").WithDetail("type", kind)
	}
	if provider.ClientSecret == "" {
		return bizerrors.ErrCaptchaInvalid.WithDetail("reason", "provider missing secret")
	}

	params := url.Values{}
	params.Set("secret", provider.ClientSecret)
	params.Set("response", token)
	if remoteIP != "" {
		params.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "failed to build captcha verification request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeServiceUnavailable, "captcha verification request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "failed to parse captcha response", err)
	}
	if !result.Success {
		return bizerrors.ErrCaptchaInvalid
	}
	return nil
}

// Test 提供商自检：本地类生成一次图片即可；远程类校验配置完整性。
func (s *CaptchaService) Test(_ context.Context, provider *repository.Provider) error {
	if kindOf(provider) == "image" {
		if _, _, _, err := s.captcha.Generate(); err != nil {
			return fmt.Errorf("本地验证码生成失败: %w", err)
		}
		return nil
	}
	if provider.ClientID == "" || provider.ClientSecret == "" {
		return fmt.Errorf("缺少 Site Key 或 Secret Key")
	}
	return nil
}
