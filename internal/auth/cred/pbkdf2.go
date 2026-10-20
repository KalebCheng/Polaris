package cred

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// pbkdf2Iterations PBKDF2-SHA256 迭代次数（对齐 Casdoor / Keycloak 约定）。
const pbkdf2Iterations = 27500

// pbkdf2DjangoIterations Django 默认迭代次数。
const pbkdf2DjangoIterations = 260000

// pbkdf2SaltManager PBKDF2-SHA256 加盐实现；salt 以 base64 编码参与运算。
type pbkdf2SaltManager struct{}

func (pbkdf2SaltManager) Type() string { return TypePbkdf2Salt }

func (pbkdf2SaltManager) Hash(password, salt string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	key := pbkdf2.Key([]byte(password), decodeSalt(salt), pbkdf2Iterations, 64, sha256.New)
	return base64.StdEncoding.EncodeToString(key), nil
}

// decodeSalt 解析盐字节：合法 base64 时按 base64 解码（兼容 Casdoor / Keycloak 既有数据），
// 否则回退为原始 UTF-8 字节（支持用户直接填写明文盐，避免"填了却不生效"）。
func decodeSalt(salt string) []byte {
	if salt == "" {
		return nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(salt); err == nil {
		return decoded
	}
	return []byte(salt)
}

func (m pbkdf2SaltManager) Verify(password, hash, salt string) bool {
	if hash == "" {
		return false
	}
	expected, _ := m.Hash(password, salt)
	return expected == hash
}

// pbkdf2DjangoManager Django 风格 PBKDF2-SHA256：哈希串自描述（pbkdf2_sha256$iters$salt$hash）。
type pbkdf2DjangoManager struct{}

func (pbkdf2DjangoManager) Type() string { return TypePbkdf2Django }

func (pbkdf2DjangoManager) Hash(password, salt string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	sum := pbkdf2.Key([]byte(password), []byte(salt), pbkdf2DjangoIterations, sha256.Size, sha256.New)
	return "pbkdf2_sha256$" + strconv.Itoa(pbkdf2DjangoIterations) + "$" + salt + "$" +
		base64.StdEncoding.EncodeToString(sum), nil
}

func (pbkdf2DjangoManager) Verify(password, hash, _ string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	sum := pbkdf2.Key([]byte(password), []byte(parts[2]), iterations, sha256.Size, sha256.New)
	return base64.StdEncoding.EncodeToString(sum) == parts[3]
}
