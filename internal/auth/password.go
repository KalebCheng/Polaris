package auth

import (
	"fmt"
	"strings"
	"unicode"

	"polaris/internal/auth/cred"
)

// HashPassword 使用默认类型（bcrypt）生成密码哈希。
// 需要按组织策略选择算法时请使用 HashPasswordWith。
func HashPassword(password string) (string, error) {
	return cred.Get(cred.DefaultType).Hash(password, "")
}

// VerifyPassword 使用默认类型（bcrypt）校验密码。
func VerifyPassword(hash, password string) bool {
	return cred.Get(cred.DefaultType).Verify(password, hash, "")
}

// HashPasswordWith 按指定密码类型与盐生成哈希（算法由 cred 包分发）。
func HashPasswordWith(password, passwordType, salt string) (string, error) {
	if password == "" {
		return "", cred.ErrEmptyPassword
	}
	return cred.Get(cred.Normalize(passwordType)).Hash(password, salt)
}

// VerifyPasswordWith 按"存储时的密码类型与盐"校验密码。
// passwordType 为空或未知时回退到默认类型（兼容历史数据，避免锁死登录）。
func VerifyPasswordWith(password, hash, passwordType, salt string) bool {
	if hash == "" {
		return false
	}
	return cred.Get(cred.Normalize(passwordType)).Verify(password, hash, salt)
}

// NormalizePasswordType 归一化密码类型：空或未知一律回退默认（bcrypt）。
func NormalizePasswordType(passwordType string) string { return cred.Normalize(passwordType) }

// SupportedPasswordTypes 返回受支持的密码类型（供服务端校验与前端下拉）。
func SupportedPasswordTypes() []string { return cred.SupportedTypes() }

// IsSupportedPasswordType 判断密码类型是否受支持。
func IsSupportedPasswordType(passwordType string) bool { return cred.IsSupported(passwordType) }

// 密码强度策略选项（与 Casdoor 对齐的枚举码）。
const (
	OptionAtLeast6    = "AtLeast6"
	OptionAtLeast8    = "AtLeast8"
	OptionAa123       = "Aa123"
	OptionSpecialChar = "SpecialChar"
	OptionNoRepeat    = "NoRepeat"
)

// defaultMinLength 默认最小长度（未显式配置长度选项时生效）。
const defaultMinLength = 8

// SupportedPasswordOptions 返回全部强度策略选项（用于服务端校验与前端多选）。
func SupportedPasswordOptions() []string {
	return []string{OptionAtLeast6, OptionAtLeast8, OptionAa123, OptionSpecialChar, OptionNoRepeat}
}

// IsSupportedPasswordOption 判断强度策略选项是否受支持。
func IsSupportedPasswordOption(option string) bool {
	for _, o := range SupportedPasswordOptions() {
		if o == option {
			return true
		}
	}
	return false
}

// PasswordPolicy 密码强度策略（对应组织级 password_options）。
type PasswordPolicy struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireDigit   bool
	RequireSpecial bool
	NoRepeat       bool
}

// DefaultPasswordPolicy 默认策略（最少 8 位）。
func DefaultPasswordPolicy() PasswordPolicy { return PasswordPolicy{MinLength: defaultMinLength} }

// FromOptions 解析组织 password_options：
//   - 优先识别枚举码（AtLeast6 / AtLeast8 / Aa123 / SpecialChar / NoRepeat）；
//   - 兼容历史自由文本（如 "At least 8 characters" / "uppercase"），避免旧数据失效。
func FromOptions(options []string) PasswordPolicy {
	p := DefaultPasswordPolicy()
	if len(options) == 0 {
		return p
	}
	explicitLength := 0
	for _, raw := range options {
		o := strings.TrimSpace(raw)
		if o == "" {
			continue
		}
		lower := strings.ToLower(o)
		switch {
		case o == OptionAtLeast6:
			explicitLength = maxInt(explicitLength, 6)
		case o == OptionAtLeast8:
			explicitLength = maxInt(explicitLength, 8)
		case o == OptionAa123:
			p.RequireUpper, p.RequireLower, p.RequireDigit = true, true, true
		case o == OptionSpecialChar:
			p.RequireSpecial = true
		case o == OptionNoRepeat:
			p.NoRepeat = true
		// —— 历史自由文本兼容 ——
		case strings.Contains(lower, "uppercase"):
			p.RequireUpper = true
		case strings.Contains(lower, "lowercase"):
			p.RequireLower = true
		case strings.Contains(lower, "digit") || strings.Contains(lower, "number"):
			p.RequireDigit = true
		case strings.Contains(lower, "special"):
			p.RequireSpecial = true
		case strings.Contains(lower, "characters"):
			explicitLength = maxInt(explicitLength, extractNumber(lower, explicitLength))
		}
	}
	if explicitLength > 0 {
		p.MinLength = explicitLength
	}
	return p
}

// Validate 校验密码是否符合策略。
func (p PasswordPolicy) Validate(password string) error {
	if len([]rune(password)) < p.MinLength {
		return fmt.Errorf("密码长度至少 %d 位", p.MinLength)
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	runes := []rune(password)
	for i, r := range runes {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
		if p.NoRepeat && i > 0 && runes[i] == runes[i-1] {
			return fmt.Errorf("密码不能包含连续重复字符")
		}
	}
	if p.RequireUpper && !hasUpper {
		return fmt.Errorf("密码需包含大写字母")
	}
	if p.RequireLower && !hasLower {
		return fmt.Errorf("密码需包含小写字母")
	}
	if p.RequireDigit && !hasDigit {
		return fmt.Errorf("密码需包含数字")
	}
	if p.RequireSpecial && !hasSpecial {
		return fmt.Errorf("密码需包含特殊字符")
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// extractNumber 从描述中提取数字（如 "at least 8 characters"）。
func extractNumber(s string, fallback int) int {
	n := 0
	found := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			found = true
		} else if found {
			break
		}
	}
	if !found || n <= 0 {
		return fallback
	}
	return n
}
