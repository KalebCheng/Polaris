package cred

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 3
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// argon2idManager Argon2id 实现，输出 PHC 标准编码串（哈希自带随机盐，忽略 salt 参数）。
//
// 编码格式：$argon2id$v=19$m=65536,t=3,p=4$<base64(salt)>$<base64(hash)>
type argon2idManager struct{}

func (argon2idManager) Type() string { return TypeArgon2id }

func (argon2idManager) Hash(password, _ string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func (argon2idManager) Verify(password, hash, _ string) bool {
	memory, time, threads, salt, key, err := decodeArgon2id(hash)
	if err != nil {
		return false
	}
	computed := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(computed, key) == 1
}

// decodeArgon2id 解析 PHC 编码串；任何格式异常都返回错误（调用方按不匹配处理）。
func decodeArgon2id(encoded string) (memory, time uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, fmt.Errorf("invalid argon2id hash")
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, fmt.Errorf("unsupported argon2id version")
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	return memory, time, threads, salt, key, nil
}
