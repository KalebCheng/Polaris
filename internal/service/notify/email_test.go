package notify

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func TestRenderEmailHTMLEscapesValuesAndKeepsMarkup(t *testing.T) {
	body := `<p>欢迎加入 {{ institution }}</p><p><a href="{{ link }}">点击登录</a></p>`
	rendered := renderEmailHTML(body, map[string]string{
		"institution": `<Acme & Co>`,
		"link":        `https://polaris.example.com/magic?token=1&next=/home`,
	})
	for _, want := range []string{
		`欢迎加入 &lt;Acme &amp; Co&gt;`,
		`href="https://polaris.example.com/magic?token=1&amp;next=/home"`,
		`<a href=`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("渲染结果缺少 %q: %s", want, rendered)
		}
	}
}

func TestEmailHeadersRejectLineBreaks(t *testing.T) {
	raw, err := buildEmailMessage(
		"Polaris <noreply@example.com>",
		"user@example.com",
		"验证码\r\nBcc: victim@example.com",
		"<p>hello</p>",
	)
	if err != nil {
		t.Fatalf("构造邮件失败: %v", err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("解析邮件失败: %v", err)
	}
	if got := message.Header.Get("Bcc"); got != "" {
		t.Fatalf("主题中的换行不应注入新头部，Bcc = %q", got)
	}
}

func TestRenderLegacyPlainTextAsParagraphs(t *testing.T) {
	rendered := renderEmailHTML("验证码 {{code}}\n5 分钟内有效\n\n请勿泄露", map[string]string{"code": "123456"})
	if !strings.Contains(rendered, "<p>验证码 123456<br>5 分钟内有效</p>") {
		t.Fatalf("历史纯文本未转换为段落: %s", rendered)
	}
	if !strings.Contains(rendered, "<p>请勿泄露</p>") {
		t.Fatalf("空行分段丢失: %s", rendered)
	}
}

func TestBuildEmailMessageUsesHTMLWithPlainTextAlternative(t *testing.T) {
	raw, err := buildEmailMessage(
		"Polaris <noreply@example.com>",
		"user@example.com",
		"登录验证码",
		`<p>您的验证码是 <strong>123456</strong></p><p><a href="https://polaris.example.com/login">登录</a></p>`,
	)
	if err != nil {
		t.Fatalf("构造邮件失败: %v", err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("解析邮件失败: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q, err = %v", message.Header.Get("Content-Type"), err)
	}
	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil || decodedSubject != "登录验证码" {
		t.Fatalf("Subject = %q, err = %v", decodedSubject, err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])

	plainPart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("读取纯文本部分失败: %v", err)
	}
	plainBody, _ := io.ReadAll(quotedprintable.NewReader(plainPart))
	if !strings.Contains(string(plainBody), "您的验证码是 123456") {
		t.Fatalf("纯文本替代内容缺失: %s", plainBody)
	}
	if !strings.Contains(string(plainBody), "https://polaris.example.com/login") {
		t.Fatalf("纯文本替代内容应保留链接地址: %s", plainBody)
	}

	htmlPart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("读取 HTML 部分失败: %v", err)
	}
	if contentType := htmlPart.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("HTML 部分 Content-Type = %q", contentType)
	}
	htmlBody, _ := io.ReadAll(quotedprintable.NewReader(htmlPart))
	if !strings.Contains(string(htmlBody), "<strong>123456</strong>") {
		t.Fatalf("HTML 部分缺失: %s", htmlBody)
	}
}
