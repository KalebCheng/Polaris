package notify

import (
	"context"
	"strings"
	"testing"
)

func TestSMTPUsesImplicitTLS(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "auto port 465", cfg: Config{Port: 465, SslMode: "auto"}, want: true},
		{name: "auto port 587", cfg: Config{Port: 587, SslMode: "auto"}, want: false},
		{name: "explicit ssl", cfg: Config{Port: 2525, SslMode: "ssl"}, want: true},
		{name: "explicit smtps", cfg: Config{Port: 2525, SslMode: "smtps"}, want: true},
		{name: "explicit starttls on 465", cfg: Config{Port: 465, SslMode: "starttls"}, want: false},
		{name: "disabled on 465", cfg: Config{Port: 465, SslMode: "disable"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smtpUsesImplicitTLS(tt.cfg); got != tt.want {
				t.Fatalf("smtpUsesImplicitTLS(%+v) = %v, want %v", tt.cfg, got, tt.want)
			}
		})
	}
}

func TestSMTPEnvelopeAddressStripsDisplayName(t *testing.T) {
	if got := smtpEnvelopeAddress("Polaris <noreply@example.com>"); got != "noreply@example.com" {
		t.Fatalf("smtpEnvelopeAddress = %q", got)
	}
	if got := smtpEnvelopeAddress(" noreply@example.com "); got != "noreply@example.com" {
		t.Fatalf("smtpEnvelopeAddress = %q", got)
	}
}

type captureEmailSender struct {
	message Message
}

func (s *captureEmailSender) Category() string { return "email" }

func (s *captureEmailSender) Send(_ context.Context, message Message) error {
	s.message = message
	return nil
}

func TestEmailProviderSelfCheckSendsTestMailWhenReceiverConfigured(t *testing.T) {
	sender := &captureEmailSender{}
	err := testEmailWithSender(
		context.Background(),
		Config{Name: "测试邮箱", Host: "smtp.qq.com", Port: 465, SslMode: "auto"},
		"receiver@example.com",
		func(Config) (Sender, error) { return sender, nil },
	)
	if err != nil {
		t.Fatalf("testEmailWithSender: %v", err)
	}
	if sender.message.To != "receiver@example.com" {
		t.Fatalf("test mail recipient = %q", sender.message.To)
	}
	if sender.message.Subject != "Polaris SMTP 连通性测试" {
		t.Fatalf("test mail subject = %q", sender.message.Subject)
	}
	if !strings.Contains(sender.message.Body, "测试邮箱") || !strings.Contains(sender.message.Body, "smtp.qq.com") {
		t.Fatalf("test mail body missing provider details: %s", sender.message.Body)
	}
}
