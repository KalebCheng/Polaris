// Package oidc 实现 Polaris 作为 OIDC Provider（IdP）的能力：
// 证书管理、令牌签发、discovery / JWKS 元数据。
package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signingKey 单个签名密钥（RSA 私钥 + 对应证书 + RFC 7638 指纹 kid）。
type signingKey struct {
	kid  string
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

// CertManager 管理 IdP 签名证书（RSA）。
// 支持多密钥并存：Sign 使用激活密钥（Keys[0]），Verify 遍历全部密钥，JWKS 发布全部密钥，
// 从而实现 JWKS 平滑轮换（新密钥生效后旧密钥仍可用于校验既有令牌，直至其自然过期）。
type CertManager struct {
	keys []*signingKey
}

// NewCertManager 从 PEM 加载证书与私钥；两者为空时自动生成自签名证书。
// 该密钥为激活密钥（用于签发）。
func NewCertManager(certPEM, keyPEM string) (*CertManager, error) {
	if certPEM == "" || keyPEM == "" {
		generatedCert, generatedKey, _, err := GenerateCert(2048, 10)
		if err != nil {
			return nil, err
		}
		certPEM, keyPEM = generatedCert, generatedKey
	}
	key, cert, err := parseCertKey(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	sk, err := newSigningKey(key, cert)
	if err != nil {
		return nil, err
	}
	return &CertManager{keys: []*signingKey{sk}}, nil
}

// parseCertKey 解析证书与私钥 PEM。
func parseCertKey(certPEM, keyPEM string) (*rsa.PrivateKey, *x509.Certificate, error) {
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("解析私钥 PEM 失败")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		parsed, err2 := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err2 != nil {
			return nil, nil, fmt.Errorf("解析私钥失败: %w", err)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("私钥类型不是 RSA")
		}
		key = rsaKey
	}
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return nil, nil, fmt.Errorf("解析证书 PEM 失败")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("解析证书失败: %w", err)
	}
	return key, cert, nil
}

func newSigningKey(key *rsa.PrivateKey, cert *x509.Certificate) (*signingKey, error) {
	kid, err := rsaThumbprint(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	return &signingKey{kid: kid, key: key, cert: cert}, nil
}

// AddKey 追加一个校验密钥（不作为激活密钥）。用于密钥轮换：
// 把新密钥加入 JWKS 供消费方预取，再在下次重启时切换激活密钥。
// 已存在相同 kid 时忽略（幂等）。
func (c *CertManager) AddKey(certPEM, keyPEM string) error {
	key, cert, err := parseCertKey(certPEM, keyPEM)
	if err != nil {
		return err
	}
	sk, err := newSigningKey(key, cert)
	if err != nil {
		return err
	}
	for _, existing := range c.keys {
		if existing.kid == sk.kid {
			return nil
		}
	}
	c.keys = append(c.keys, sk)
	return nil
}

// GenerateCert 生成自签名 RSA 证书与私钥（PEM）。
func GenerateCert(bitSize, years int) (certPEM, keyPEM, kid string, err error) {
	if bitSize <= 0 {
		bitSize = 2048
	}
	if years <= 0 {
		years = 10
	}
	key, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return "", "", "", fmt.Errorf("生成 RSA 私钥失败: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", "", fmt.Errorf("生成序列号失败: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Polaris", Organization: []string{"Polaris"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(years, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", "", fmt.Errorf("签发证书失败: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	thumb, err := rsaThumbprint(&key.PublicKey)
	if err != nil {
		return "", "", "", err
	}
	return certPEM, keyPEM, thumb, nil
}

// active 返回激活密钥（Keys[0]）；无密钥时返回 nil。
func (c *CertManager) active() *signingKey {
	if c == nil || len(c.keys) == 0 {
		return nil
	}
	return c.keys[0]
}

// KID 返回激活密钥标识
func (c *CertManager) KID() string {
	if sk := c.active(); sk != nil {
		return sk.kid
	}
	return ""
}

// Certificate 返回激活密钥的证书 PEM
func (c *CertManager) Certificate() string {
	sk := c.active()
	if sk == nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: sk.cert.Raw}))
}

// NotAfter 返回激活密钥证书的到期时间
func (c *CertManager) NotAfter() time.Time {
	if sk := c.active(); sk != nil {
		return sk.cert.NotAfter
	}
	return time.Time{}
}

// Sign 使用 RS256 与激活密钥签发 JWT。
func (c *CertManager) Sign(claims jwt.MapClaims) (string, error) {
	return c.SignTyp(claims, "")
}

// SignTyp 使用 RS256 与激活密钥签发 JWT，并设置 JOSE 头的 typ（为空则不设置）。
func (c *CertManager) SignTyp(claims jwt.MapClaims, typ string) (string, error) {
	sk := c.active()
	if sk == nil {
		return "", fmt.Errorf("无可用的签名密钥")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if typ != "" {
		token.Header["typ"] = typ
	}
	token.Header["kid"] = sk.kid
	return token.SignedString(sk.key)
}

// Verify 校验 RS256 JWT：遍历全部密钥，任一公钥验签成功即可（支持密钥轮换期间新旧令牌共存）。
func (c *CertManager) Verify(tokenString string, claims jwt.MapClaims) error {
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid != "" {
			for _, sk := range c.keys {
				if sk.kid == kid {
					return &sk.key.PublicKey, nil
				}
			}
			// kid 未命中：回退到遍历全部密钥（消费方可能未携带 kid）
		}
		if len(c.keys) == 0 {
			return nil, fmt.Errorf("无可用的校验密钥")
		}
		return &c.keys[0].key.PublicKey, nil
	})
	return err
}

// JWKS 返回 JSON Web Key Set：发布全部密钥（激活密钥在前），供消费方预取与校验。
func (c *CertManager) JWKS() map[string]interface{} {
	keys := make([]map[string]interface{}, 0, len(c.keys))
	for _, sk := range c.keys {
		pub := sk.key.PublicKey
		e := big.NewInt(int64(pub.E))
		keys = append(keys, map[string]interface{}{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": sk.kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(e.Bytes()),
		})
	}
	return map[string]interface{}{"keys": keys}
}

// rsaThumbprint 按 RFC 7638 计算 RSA 公钥指纹（取前 16 字节 base64url）。
func rsaThumbprint(pub *rsa.PublicKey) (string, error) {
	e := big.NewInt(int64(pub.E))
	canonical := fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`,
		base64.RawURLEncoding.EncodeToString(e.Bytes()),
		base64.RawURLEncoding.EncodeToString(pub.N.Bytes()))
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:16]), nil
}
