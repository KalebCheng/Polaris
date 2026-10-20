package cred

import (
	"crypto/md5"
	"encoding/hex"
)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// md5SaltManager MD5 加盐实现（仅用于兼容历史/外部数据，安全性弱，不建议新启用）。
//
// 计算方式：salt 为空 → Hex(MD5(password))；否则 → Hex(MD5(Hex(MD5(password)) + salt))。
type md5SaltManager struct{}

func (md5SaltManager) Type() string { return TypeMD5Salt }

func (md5SaltManager) Hash(password, salt string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	if salt == "" {
		return md5Hex(password), nil
	}
	return md5Hex(md5Hex(password) + salt), nil
}

func (m md5SaltManager) Verify(password, hash, salt string) bool {
	if hash == "" {
		return false
	}
	expected, _ := m.Hash(password, salt)
	if expected == hash {
		return true
	}
	if salt == "" {
		return md5Hex(md5Hex(password)) == hash
	}
	return false
}
