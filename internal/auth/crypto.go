package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// SecretCipher AES-256-GCM 对称加密器（用于 TOTP 密钥、同步器口令等敏感字段落库）。
type SecretCipher struct {
	aead cipher.AEAD
}

// NewSecretCipher 使用 32 字节密钥创建加密器。
func NewSecretCipher(key []byte) (*SecretCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("加密密钥必须为 32 字节（AES-256），实际 %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化 GCM 失败: %w", err)
	}
	return &SecretCipher{aead: aead}, nil
}

// DeriveKey 由任意长度的口令派生 32 字节密钥（SHA-256）。
func DeriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Encrypt 加密明文，返回 base64 密文。
func (c *SecretCipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成 nonce 失败: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密 base64 密文。
func (c *SecretCipher) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("解码密文失败: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("密文长度非法")
	}
	plain, err := c.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}
	return string(plain), nil
}
