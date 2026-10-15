package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPIssuer TOTP 发行方名称（展示在验证器 App 中）
const TOTPIssuer = "Polaris"

// GenerateTOTPSecret 生成 TOTP 密钥与 otpauth URL。
//
// 标签（验证器 App 里显示的那一行）约定为 `<用户名>@<组织标识>`，发行方为平台名：
//   - 不含组织主键：既无意义也难读（历史实现是 `Polaris:2/ceshi@Polaris`，证书器里显示成
//     「Polaris (2/ceshi@Polaris)」）；
//   - 带组织标识是为了区分"同名用户分属不同组织"的场景（同名账号在多个组织是允许的）。
func GenerateTOTPSecret(institution, userName string) (secret string, url string, err error) {
	account := userName
	if strings.TrimSpace(institution) != "" {
		account = fmt.Sprintf("%s@%s", userName, institution)
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
		AccountName: account,
	})
	if err != nil {
		return "", "", fmt.Errorf("生成 TOTP 密钥失败: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// VerifyTOTP 校验 TOTP 动态码。
func VerifyTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, nowTime(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes 生成一次性恢复码。
func GenerateRecoveryCodes(n int) ([]string, error) {
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		buf := make([]byte, 5)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("生成恢复码失败: %w", err)
		}
		codes = append(codes, hex.EncodeToString(buf))
	}
	return codes, nil
}
