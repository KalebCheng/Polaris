package cred

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sha512Hex(s string) string {
	sum := sha512.Sum512([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sha256SaltManager SHA-256 加盐实现（Casdoor 的 "salt" 类型）。
//
// 计算方式：salt 为空 → Hex(SHA256(password))；否则 → Hex(SHA256(Hex(SHA256(password)) + salt))。
type sha256SaltManager struct{}

func (sha256SaltManager) Type() string { return TypeSHA256Salt }

func (sha256SaltManager) Hash(password, salt string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	if salt == "" {
		return sha256Hex(password), nil
	}
	return sha256Hex(sha256Hex(password) + salt), nil
}

func (m sha256SaltManager) Verify(password, hash, salt string) bool {
	if hash == "" {
		return false
	}
	expected, _ := m.Hash(password, salt)
	if expected == hash {
		return true
	}
	// 向后兼容：无盐场景同时接受对「已哈希值」再求哈希的历史数据。
	if salt == "" {
		return sha256Hex(sha256Hex(password)) == hash
	}
	return false
}

// sha512SaltManager SHA-512 加盐实现，规则同 SHA-256。
type sha512SaltManager struct{}

func (sha512SaltManager) Type() string { return TypeSHA512Salt }

func (sha512SaltManager) Hash(password, salt string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	if salt == "" {
		return sha512Hex(password), nil
	}
	return sha512Hex(sha512Hex(password) + salt), nil
}

func (m sha512SaltManager) Verify(password, hash, salt string) bool {
	if hash == "" {
		return false
	}
	expected, _ := m.Hash(password, salt)
	if expected == hash {
		return true
	}
	if salt == "" {
		return sha512Hex(sha512Hex(password)) == hash
	}
	return false
}
