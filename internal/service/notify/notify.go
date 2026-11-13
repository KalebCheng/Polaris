// Package notify 提供"通知类提供商"的实际发送能力：邮件（SMTP）与短信（阿里云 / 通用 HTTP）。
//
// 设计与提供商模块的关系：providers 表按 category 保存配置，本包按 category 构造具体发送器；
// 业务层只依赖 Sender 接口，新增类别只需实现接口并注册。
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
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
	// TemplateParams 厂商模板变量 → 平台占位符映射；空表示只发 code（历史数据兜底）
	TemplateParams map[string]string
	AppID          string
	AppSecret      string
	Endpoint       string
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

type senderFactory func(Config) (Sender, error)

var senderStrategies = map[string]senderFactory{
	"email": newEmailSender,
	"sms":   newSMSSender,
}

var smsSenderStrategies = map[string]senderFactory{
	"aliyun":  newAliyunSMSSender,
	"":        newAliyunSMSSender,
	"http":    newHTTPSMSSender,
	"generic": newHTTPSMSSender,
}

type testStrategy func(context.Context, Config, string) error

var testStrategies = map[string]testStrategy{
	"email": testEmail,
	"sms":   testSMS,
}

// New 依据提供商配置构造发送器；category 为 email 或 sms。
func New(category string, cfg Config) (Sender, error) {
	factory, ok := senderStrategies[strings.ToLower(strings.TrimSpace(category))]
	if !ok {
		return nil, fmt.Errorf("不支持的提供商类别: %s", category)
	}
	return factory(cfg)
}

func newEmailSender(cfg Config) (Sender, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("邮件提供商缺少 SMTP 主机")
	}
	return &EmailSender{cfg: cfg}, nil
}

func newSMSSender(cfg Config) (Sender, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Type))
	factory, ok := smsSenderStrategies[kind]
	if !ok {
		return nil, fmt.Errorf("不支持的短信提供商类型: %s", cfg.Type)
	}
	return factory(cfg)
}

func newAliyunSMSSender(cfg Config) (Sender, error) {
	if cfg.SignName == "" || cfg.TemplateCode == "" {
		return nil, fmt.Errorf("短信提供商缺少签名或模板编号")
	}
	return &AliyunSMSSender{cfg: cfg}, nil
}

func newHTTPSMSSender(cfg Config) (Sender, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("短信提供商缺少自定义发送地址")
	}
	return &HTTPSMSSender{cfg: cfg}, nil
}

var notificationPlaceholderPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// render 渲染模板中的 {{key}} 占位符；允许占位符内出现空白。
func render(template string, vars map[string]string) string {
	return renderPlaceholders(template, vars, false)
}

func renderPlaceholders(template string, vars map[string]string, escape bool) string {
	return notificationPlaceholderPattern.ReplaceAllStringFunc(template, func(token string) string {
		match := notificationPlaceholderPattern.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		value, ok := vars[strings.TrimSpace(match[1])]
		if !ok {
			return token
		}
		if escape {
			return html.EscapeString(value)
		}
		return value
	})
}

var emailHTMLTagPattern = regexp.MustCompile(`(?i)<[a-z][^>]*>`)

// renderEmailHTML 渲染邮件正文：富文本按 HTML 输出，历史纯文本按段落转义后输出。
// 变量值一律 HTML 转义，避免组织名/应用名里的尖括号改变正文结构。
func renderEmailHTML(template string, vars map[string]string) string {
	plain := !emailHTMLTagPattern.MatchString(template)
	out := template
	if plain {
		out = html.EscapeString(out)
	}
	out = renderPlaceholders(out, vars, true)
	if !plain {
		return out
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	blocks := strings.Split(out, "\n\n")
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		parts = append(parts, "<p>"+strings.ReplaceAll(block, "\n", "<br>")+"</p>")
	}
	return strings.Join(parts, "")
}

// EmailSender 基于 net/smtp 的邮件发送器
type EmailSender struct{ cfg Config }

// Category 类别
func (s *EmailSender) Category() string { return "email" }

// Send 发送邮件
func (s *EmailSender) Send(ctx context.Context, msg Message) error {
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
	subject = render(subject, series)
	body = renderEmailHTML(body, series)
	if strings.TrimSpace(body) == "" {
		body = fmt.Sprintf("<p>您的验证码是 <strong>%s</strong>，5 分钟内有效。</p>", html.EscapeString(msg.Code))
	}

	raw, err := buildEmailMessage(from, msg.To, subject, body)
	if err != nil {
		return err
	}

	client, err := dialSMTP(ctx, s.cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if s.cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	fromAddress := smtpEnvelopeAddress(from)
	toAddress := smtpEnvelopeAddress(msg.To)
	if fromAddress == "" || toAddress == "" {
		return fmt.Errorf("邮件地址格式无效")
	}
	if err := client.Mail(fromAddress); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	if err := client.Rcpt(toAddress); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("开始写入邮件正文失败: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("写入邮件正文失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("结束邮件正文失败: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("结束 SMTP 会话失败: %w", err)
	}
	return nil
}

// buildEmailMessage 构造 multipart/alternative 邮件：客户端优先渲染 HTML，
// 不支持 HTML 的客户端读取纯文本替代部分。主题按 RFC 2047 编码，避免中文乱码。
func buildEmailMessage(from, to, subject, htmlBody string) ([]byte, error) {
	var buf bytes.Buffer
	from = emailHeaderValue(from)
	to = emailHeaderValue(to)
	subject = emailHeaderValue(subject)
	fromHeader := from
	if address, err := mail.ParseAddress(from); err == nil {
		fromHeader = address.String()
	}
	_, _ = fmt.Fprintf(&buf, "From: %s\r\n", fromHeader)
	_, _ = fmt.Fprintf(&buf, "To: %s\r\n", to)
	_, _ = fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("X-Mailer: Polaris\r\n")

	writer := multipart.NewWriter(&buf)
	_, _ = fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", writer.Boundary())
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	textHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	textPart, err := writer.CreatePart(textHeader)
	if err != nil {
		return nil, err
	}
	textWriter := quotedprintable.NewWriter(textPart)
	if _, err := textWriter.Write([]byte(emailPlainText(htmlBody) + "\r\n")); err != nil {
		return nil, err
	}
	if err := textWriter.Close(); err != nil {
		return nil, err
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return nil, err
	}
	htmlWriter := quotedprintable.NewWriter(htmlPart)
	if _, err := htmlWriter.Write([]byte(htmlBody)); err != nil {
		return nil, err
	}
	if err := htmlWriter.Close(); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func emailHeaderValue(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

// emailPlainText 从 HTML 正文提取纯文本替代内容；链接保留文字与目标地址。
func emailPlainText(body string) string {
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(strings.NewReader(body), context)
	if err != nil {
		return body
	}
	var out strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		switch node.Type {
		case xhtml.TextNode:
			out.WriteString(node.Data)
			return
		case xhtml.ElementNode:
			tag := strings.ToLower(node.Data)
			if tag == "br" {
				out.WriteString("\n")
				return
			}
			block := tag == "p" || tag == "div" || tag == "h1" || tag == "h2" ||
				tag == "h3" || tag == "li" || tag == "blockquote" || tag == "hr"
			if block {
				out.WriteString("\n")
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if tag == "a" {
				href := emailAttr(node, "href")
				if href != "" && !strings.Contains(href, "{{") {
					out.WriteString(" (")
					out.WriteString(href)
					out.WriteString(")")
				}
			}
			if block {
				out.WriteString("\n")
			}
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	lines := strings.Split(out.String(), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func emailAttr(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
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
	for name, value := range templateParams(cfg, msg) {
		if _, fixed := payload[name]; fixed {
			continue
		}
		payload[name] = value
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
	defer func() { _ = resp.Body.Close() }()
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
	strategy, ok := testStrategies[strings.ToLower(strings.TrimSpace(category))]
	if !ok {
		return fmt.Errorf("不支持的提供商类别: %s", category)
	}
	return strategy(ctx, cfg, target)
}

func testEmail(ctx context.Context, cfg Config, target string) error {
	return testEmailWithSender(ctx, cfg, target, newEmailSender)
}

// testEmailWithSender 有自检收件人时真实发送一封测试邮件；否则只验证连接与认证。
// senderFactory 作为参数传入，便于回归测试锁定"用户填了收件人就一定会发信"。
func testEmailWithSender(ctx context.Context, cfg Config, target string, factory senderFactory) error {
	if strings.TrimSpace(target) == "" {
		return testSMTP(ctx, cfg)
	}
	sender, err := factory(cfg)
	if err != nil {
		return err
	}
	return sender.Send(ctx, Message{
		To:      target,
		Subject: "Polaris SMTP 连通性测试",
		Body: fmt.Sprintf(
			"<p>这是一封来自 Polaris 的 SMTP 连通性测试邮件。</p><p>提供商：%s<br>SMTP：%s</p>",
			html.EscapeString(cfg.Name), html.EscapeString(cfg.Host),
		),
	})
}

func testSMS(ctx context.Context, cfg Config, target string) error {
	sender, err := New("sms", cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(target) == "" {
		return nil
	}
	return sender.Send(ctx, Message{
		To: target, Code: "000000",
		Vars: map[string]string{
			"code": "000000", "minutes": "5", "phone": target,
			"institution": "Polaris", "application": "Polaris",
		},
	})
}

// testSMTP 建立 SMTP 连接并认证，用于管理端"测试"动作。
func testSMTP(ctx context.Context, cfg Config) error {
	client, err := dialSMTP(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("结束 SMTP 会话失败: %w", err)
	}
	return nil
}

// dialSMTP 统一连接入口：465 使用隐式 TLS，其余端口按 ssl_mode 走 STARTTLS。
// QQ / 163 等邮箱的 465 端口要求先完成 TLS 握手再发 EHLO，明文连接会被直接断开（EOF）。
func dialSMTP(ctx context.Context, cfg Config) (*smtp.Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("缺少 SMTP 主机")
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(orDefaultInt(cfg.Port, 587)))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	var (
		conn net.Conn
		err  error
	)
	implicitTLS := smtpUsesImplicitTLS(cfg)
	if implicitTLS {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsConfig}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	if implicitTLS {
		return client, nil
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.SslMode))
	if ok, _ := client.Extension("STARTTLS"); ok && mode != "disable" {
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("STARTTLS 失败: %w", err)
		}
	} else if mode == "starttls" {
		_ = client.Close()
		return nil, fmt.Errorf("SMTP 服务器未提供 STARTTLS")
	}
	return client, nil
}

func smtpUsesImplicitTLS(cfg Config) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.SslMode)) {
	case "ssl", "tls", "smtps":
		return true
	case "starttls", "disable":
		return false
	default:
		return orDefaultInt(cfg.Port, 587) == 465
	}
}

func smtpEnvelopeAddress(value string) string {
	value = strings.TrimSpace(value)
	if address, err := mail.ParseAddress(value); err == nil {
		return address.Address
	}
	return value
}
