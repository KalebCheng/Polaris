package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AliyunSMSSender 阿里云短信（RPC 风格 OpenAPI，HMAC-SHA1 签名）。
// 依据阿里云"短信服务"公共参数规范实现，无需引入官方 SDK。
type AliyunSMSSender struct{ cfg Config }

// Category 类别
func (s *AliyunSMSSender) Category() string { return "sms" }

// Send 调用 SendSms 接口
func (s *AliyunSMSSender) Send(ctx context.Context, msg Message) error {
	if s.cfg.AppID == "" || s.cfg.AppSecret == "" {
		return fmt.Errorf("阿里云短信缺少 AccessKey")
	}
	params := map[string]string{
		"AccessKeyId":      s.cfg.AppID,
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     msg.To,
		"RegionId":         orDefault(s.cfg.RegionID, "cn-hangzhou"),
		"SignName":         s.cfg.SignName,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"TemplateCode":     s.cfg.TemplateCode,
		"TemplateParam":    mustJSON(templateParams(s.cfg, msg)),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
	}
	params["Signature"] = signAliyun(params, s.cfg.AppSecret)

	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	endpoint := orDefault(s.cfg.Endpoint, "https://dysmsapi.aliyuncs.com")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("阿里云短信请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	_ = json.Unmarshal(body, &result)
	if resp.StatusCode != http.StatusOK || (result.Code != "" && result.Code != "OK") {
		return fmt.Errorf("阿里云短信返回 %d/%s: %s", resp.StatusCode, result.Code, result.Message)
	}
	return nil
}

// templateParams 按模板声明的变量装配 TemplateParam。
//
// 阿里云要求 key 与模板内容里的 ${变量} 严格一致：多一个、少一个、名字对不上都会被拒。
// 因此这里只放**声明过的**变量；未声明时退回历史行为（只发 code）。
func templateParams(cfg Config, msg Message) map[string]string {
	names := cfg.TemplateParams
	if len(names) == 0 {
		return map[string]string{"code": msg.Code}
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		switch name {
		case "code":
			out[name] = msg.Code
		case "phone":
			out[name] = msg.To
		default:
			if value, ok := msg.Vars[name]; ok {
				out[name] = value
			}
		}
	}
	return out
}

// signAliyun 按阿里云签名规范对参数签名（GET/POST 表单通用）。
func signAliyun(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if key == "Signature" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for i, key := range keys {
		if i > 0 {
			builder.WriteString("&")
		}
		builder.WriteString(percentEncode(key))
		builder.WriteString("=")
		builder.WriteString(percentEncode(params[key]))
	}
	stringToSign := "POST&%2F&" + percentEncode(builder.String())
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// percentEncode 阿里云要求的 RFC 3986 编码（空格为 %20，* 保持原样，~ 不转义）
func percentEncode(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func mustJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
