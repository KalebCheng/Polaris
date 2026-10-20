// Package cred 实现「密码凭据管理器」抽象：把明文密码（可结合盐）转换为可存储哈希，并做校验。
//
// 设计目标（对齐 Casdoor 的 cred 包，保证算法兼容）：
//   - 组织（institution）通过 password_type 选择算法、password_salt 提供组织级盐；
//   - 上层（用户服务 / 认证服务 / OIDC）只依赖 Manager 接口，不感知具体算法；
//   - 新增算法只需实现 Manager 并在 registry 注册，调用方零改动。
//
// 盐的语义（各实现自行决定是否使用 salt 参数）：
//   - 随机盐算法（bcrypt / argon2id）：哈希自体携带随机盐，salt 参数被忽略；
//   - 固定盐算法（salt / sha512-salt / md5-salt / pbkdf2-salt）：salt 参与运算，盐为空则退化为无盐哈希；
//   - pbkdf2-django：哈希串自描述（pbkdf2_sha256$iters$salt$hash），salt 参数被忽略；
//   - plain：明文存储，仅用于兼容外部导入数据，平台默认不启用。
package cred

import "errors"

// 密码类型标识（与组织 password_type 取值一致）
const (
	TypePlain        = "plain"
	TypeBcrypt       = "bcrypt"
	TypeArgon2id     = "argon2id"
	TypeSHA256Salt   = "salt" // Casdoor 的 "salt" 即 SHA-256 加盐
	TypeSHA512Salt   = "sha512-salt"
	TypeMD5Salt      = "md5-salt"
	TypePbkdf2Salt   = "pbkdf2-salt"
	TypePbkdf2Django = "pbkdf2-django"
)

// DefaultType 默认密码类型（未知/空值一律回退到它）
const DefaultType = TypeBcrypt

// ErrEmptyPassword 空密码不可哈希
var ErrEmptyPassword = errors.New("密码不能为空")

// Manager 密码凭据管理器
type Manager interface {
	// Type 返回该管理器对应的密码类型标识
	Type() string
	// Hash 生成可存储的密码哈希（是否需要 salt 由实现决定）
	Hash(password, salt string) (string, error)
	// Verify 校验明文密码与哈希是否匹配
	Verify(password, hash, salt string) bool
}

// registry 全部受支持的密码类型实现（map 保证单点注册）
var registry = map[string]Manager{
	TypePlain:        plainManager{},
	TypeBcrypt:       bcryptManager{},
	TypeArgon2id:     argon2idManager{},
	TypeSHA256Salt:   sha256SaltManager{},
	TypeSHA512Salt:   sha512SaltManager{},
	TypeMD5Salt:      md5SaltManager{},
	TypePbkdf2Salt:   pbkdf2SaltManager{},
	TypePbkdf2Django: pbkdf2DjangoManager{},
}

// supportedOrder 受支持类型的展示/校验顺序（推荐在前）
var supportedOrder = []string{
	TypeBcrypt, TypeArgon2id, TypePbkdf2Salt, TypePbkdf2Django,
	TypeSHA256Salt, TypeSHA512Salt, TypeMD5Salt, TypePlain,
}

// Get 按密码类型获取管理器；未知类型回退到默认实现（避免配置异常导致登录整体不可用）。
func Get(passwordType string) Manager {
	if m, ok := registry[passwordType]; ok {
		return m
	}
	return registry[DefaultType]
}

// SupportedTypes 返回全部受支持的密码类型（用于服务端校验与前端下拉）。
func SupportedTypes() []string {
	out := make([]string, len(supportedOrder))
	copy(out, supportedOrder)
	return out
}

// IsSupported 判断密码类型是否受支持。
func IsSupported(passwordType string) bool {
	_, ok := registry[passwordType]
	return ok
}

// Normalize 归一化密码类型：空或未知一律返回默认类型。
func Normalize(passwordType string) string {
	if IsSupported(passwordType) {
		return passwordType
	}
	return DefaultType
}
