package auth

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestGenerateAndVerifyTOTP(t *testing.T) {
	secret, url, err := GenerateTOTPSecret("built-in", "admin")
	if err != nil {
		t.Fatalf("GenerateTOTPSecret 失败: %v", err)
	}
	if secret == "" || url == "" {
		t.Fatal("密钥与 otpauth URL 均不应为空")
	}
	code, err := totp.GenerateCodeCustom(secret, time.Now(), totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("生成动态码失败: %v", err)
	}
	if !VerifyTOTP(secret, code) {
		t.Fatal("正确动态码应校验通过")
	}
	if VerifyTOTP(secret, "000000") {
		t.Fatal("错误动态码不应通过")
	}
	if VerifyTOTP("", code) || VerifyTOTP(secret, "") {
		t.Fatal("空密钥或空码不应通过")
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes 失败: %v", err)
	}
	if len(codes) != 8 {
		t.Fatalf("恢复码数量 = %d, 期望 8", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 10 {
			t.Fatalf("恢复码长度 = %d, 期望 10", len(c))
		}
		if seen[c] {
			t.Fatalf("恢复码重复: %s", c)
		}
		seen[c] = true
	}
}

// 回归：otpauth 标签必须可读——`<用户名>@<组织标识>`，由平台名做发行方。
//
// 历史实现是 `Issuer=Polaris` + `AccountName=2/ceshi@Polaris`，
// 验证器 App 里显示成「Polaris (2/ceshi@Polaris)」：混进了组织主键，且重复了一次平台名。
func TestGenerateTOTPSecretLabelIsReadable(t *testing.T) {
	secret, raw, err := GenerateTOTPSecret("测试", "ceshi")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("应返回密钥")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("otpauth URL 无法解析: %v", err)
	}
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" {
		t.Fatalf("应为 otpauth://totp 链接: %s", raw)
	}
	label := strings.TrimPrefix(parsed.Path, "/")
	if label != "Polaris:ceshi@测试" {
		t.Fatalf("标签 = %q, want %q", label, "Polaris:ceshi@测试")
	}
	if strings.Contains(label, "/") {
		t.Fatalf("标签不应包含组织主键/斜杠: %q", label)
	}
	query := parsed.Query()
	if query.Get("issuer") != TOTPIssuer {
		t.Fatalf("issuer 参数应为平台名: %s", raw)
	}
	if query.Get("secret") == "" {
		t.Fatalf("缺少 secret: %s", raw)
	}

	// 未提供组织标识时退化为纯用户名（不出现多余分隔符）
	_, raw2, err := GenerateTOTPSecret("", "ceshi")
	if err != nil {
		t.Fatal(err)
	}
	parsed2, _ := url.Parse(raw2)
	if label2 := strings.TrimPrefix(parsed2.Path, "/"); label2 != "Polaris:ceshi" {
		t.Fatalf("无组织时标签 = %q, want %q", label2, "Polaris:ceshi")
	}
}
