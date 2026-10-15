// Package notify 提供"通知类提供商"的实际发送能力：邮件（SMTP）与短信（阿里云 / 通用 HTTP）。
//
// 设计与提供商模块的关系：providers 表按 category 保存配置，本包按 category 构造具体发送器；
// 业务层只依赖 Sender 接口，新增类别只需实现接口并注册。
package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message 一条待发送的通知
type Message struct {
	To      string            // 收件人（邮箱或手机号）
	Subject string            // 邮件主题（短信忽略）
	Body    string            // 正文
	Code    string            // 验证码（模板占位符）
	Vars    map[string]string // 其他模板变量
}

// Sender 通知发送器
type Sender interface {
	// Category 提供商类别（email / sms）
	Category() string
	// Send 发送消息
	Send(ctx context.Context, msg Message) error
}

// emailConfig 与 SMS 配置均来自 providers 表的字段，这里用最小结构承载，避免依赖 repository 包造成循环引用。
type Config struct {
	Name         string
	Type         string // smtp | aliyun | http
	Host         string
	Port         int
	User         string
	Password     string
	SslMode      string
	FromAddress  string
	Title        string
	Content      string
	RegionID     string
	SignName     string
	TemplateCode string
	// TemplateParams 模板变量声明（内置变量集的子集）；空表示只发 code（历史行为）
	TemplateParams []string
	AppID          string
	AppSecret      string
	Endpoint       string
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// New 依据提供商配置构造发送器；category 为 email 或 sms。
func New(category string, cfg Config) (Sender, error) {
	switch strings.ToLower(category) {
	case "email":
		if cfg.Host == "" {
			return nil, fmt.Errorf("邮件提供商缺少 SMTP 主机")
		}
		return &EmailSender{cfg: cfg}, nil
	case "sms":
		switch strings.ToLower(cfg.Type) {
		case "aliyun", "":
			if cfg.SignName == "" || cfg.TemplateCode == "" {
				return nil, fmt.Errorf("短信提供商缺少签名或模板编号")
			}
			return &AliyunSMSSender{cfg: cfg}, nil
		case "http", "generic":
			if cfg.Endpoint == "" {
				return nil, fmt.Errorf("短信提供商缺少自定义发送地址")
			}
			return &HTTPSMSSender{cfg: cfg}, nil
		default:
			return nil, fmt.Errorf("不支持的短信提供商类型: %s", cfg.Type)
		}
	default:
		return nil, fmt.Errorf("不支持的提供商类别: %s", category)
	}
}

// render 渲染模板中的 {{key}} 占位符
func render(template string, vars map[string]string) string {
	out := template
	for key, value := range vars {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
	}
	return out
}

// EmailSender 基于 net/smtp 的邮件发送器
type EmailSender struct{ cfg Config }

// Category 类别
func (s *EmailSender) Category() string { return "email" }

// Send 发送邮件
func (s *EmailSender) Send(_ context.Context, msg Message) error {
	from := s.cfg.FromAddress
	if from == "" {
		from = s.cfg.User
	}
	if from == "" {
		return fmt.Errorf("邮件提供商缺少发信人地址")
	}
	subject := msg.Subject
	if subject == "" {
		subject = s.cfg.Title
	}
	if subject == "" {
		subject = "Polaris 通知"
	}
	body := msg.Body
	if body == "" {
		body = s.cfg.Content
	}
	series := map[string]string{"code": msg.Code, "to": msg.To}
	for k, v := range msg.Vars {
		series[k] = v
	}
	body = render(body, series)
	if strings.TrimSpace(body) == "" {
		body = fmt.Sprintf("您的验证码是 %s，5 分钟内有效。", msg.Code)
	}

	host := s.cfg.Host
	// 使用 net.JoinHostPort 以兼容 IPv6 字面量主机
	addr := net.JoinHostPort(host, strconv.Itoa(orDefaultInt(s.cfg.Port, 587)))
	header := map[string]string{
		"From":         from,
		"To":           msg.To,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=UTF-8",
	}
	var builder strings.Builder
	for k, v := range header {
		builder.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	builder.WriteString("\r\n")
	builder.WriteString(body)

	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Password, host)
	}
	return smtp.SendMail(addr, auth, from, []string{msg.To}, []byte(builder.String()))
}

// smsGatewayPayload 通用 HTTP 网关的请求体。
//
// 固定字段（phone / code / signName / template）是历史契约，保持不变；
// 模板**声明过**的额外变量（如 minutes）追加进来，因此声明式参数对通用网关同样生效。
func smsGatewayPayload(cfg Config, msg Message) map[string]string {
	payload := map[string]string{
		"phone":    msg.To,
		"code":     msg.Code,
		"signName": cfg.SignName,
		"template": cfg.TemplateCode,
	}
	for name := range templateParams(cfg, msg) {
		if _, fixed := payload[name]; fixed {
			continue
		}
		if value, ok := msg.Vars[name]; ok {
			payload[name] = value
		}
	}
	return payload
}

// HTTPSMSSender 通用 HTTP 短信网关：以 JSON 形式 POST 到自定义地址
type HTTPSMSSender struct{ cfg Config }

// Category 类别
func (s *HTTPSMSSender) Category() string { return "sms" }

// Send 调用自定义短信网关
func (s *HTTPSMSSender) Send(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(smsGatewayPayload(s.cfg, msg))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("短信网关请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("短信网关返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func orDefaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

// Test 连通性自检：
//   - 邮件：建立连接并完成认证（不发送真实邮件）；
//   - 短信：配置了 receiver 时真实发送一条测试短信，否则仅校验配置完整性。
func Test(ctx context.Context, category string, cfg Config, target string) error {
	switch strings.ToLower(category) {
	case "email":
		return testSMTP(cfg)
	case "sms":
		sender, err := New(category, cfg)
		if err != nil {
			return err
		}
		if strings.TrimSpace(target) == "" {
			return nil // 无测试目标时仅校验配置
		}
		return sender.Send(ctx, Message{To: target, Code: "000000"})
	default:
		return fmt.Errorf("不支持的提供商类别: %s", category)
	}
}

// testSMTP 建立 SMTP 连接并认证，用于管理端"测试"动作
func testSMTP(cfg Config) error {
	if cfg.Host == "" {
		return fmt.Errorf("缺少 SMTP 主机")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(orDefaultInt(cfg.Port, 587)))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok && strings.ToLower(cfg.SslMode) != "disable" {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}
	if cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	return nil
}
