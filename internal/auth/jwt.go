package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT 声明结构。
// 红线：JWT 仅携带 session ID，完整认证信息存储在 Redis 会话中。
type JWTClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

// MfaTrustClaims 「信任设备（记住此设备）」令牌声明。
// 与会话令牌分离：仅用于免二次校验判定，不作为会话凭证。
type MfaTrustClaims struct {
	InstitutionID int64  `json:"inst"`
	User          string `json:"usr"`
	jwt.RegisteredClaims
}

// JWTManager JWT 管理器
type JWTManager struct {
	secret     []byte
	issuer     string
	expiryTime time.Duration
}

// NewJWTManager 创建 JWT 管理器
func NewJWTManager(secret, issuer string, expirySeconds int) *JWTManager {
	if issuer == "" {
		issuer = "polaris"
	}
	return &JWTManager{secret: []byte(secret), issuer: issuer, expiryTime: time.Duration(expirySeconds) * time.Second}
}

// Generate 生成 JWT（仅含 session ID）
func (m *JWTManager) Generate(sessionID string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   sessionID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiryTime)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("签名 JWT 失败: %w", err)
	}
	return signed, nil
}

// Parse 解析并校验 JWT
func (m *JWTManager) Parse(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		if err == jwt.ErrTokenExpired {
			return nil, fmt.Errorf("token 已过期")
		}
		return nil, fmt.Errorf("解析 token 失败: %w", err)
	}
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("无效的 token 声明")
}

// Issuer 返回签发者标识
func (m *JWTManager) Issuer() string { return m.issuer }

// GenerateMfaTrust 签发「信任设备」令牌（TTL 为组织的记住时长）。
func (m *JWTManager) GenerateMfaTrust(institutionID int64, user string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := MfaTrustClaims{
		InstitutionID: institutionID,
		User:          user,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatInt(institutionID, 10) + ":" + user,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("签名信任设备令牌失败: %w", err)
	}
	return signed, nil
}

// VerifyMfaTrust 校验「信任设备」令牌是否对该机构/用户有效（过期、篡改、不匹配均为无效）。
func (m *JWTManager) VerifyMfaTrust(tokenString string, institutionID int64, user string) bool {
	if tokenString == "" {
		return false
	}
	token, err := jwt.ParseWithClaims(tokenString, &MfaTrustClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return false
	}
	claims, ok := token.Claims.(*MfaTrustClaims)
	if !ok || !token.Valid {
		return false
	}
	return claims.InstitutionID == institutionID && claims.User == user
}
