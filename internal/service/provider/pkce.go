package provider

import "golang.org/x/oauth2"

// NewPKCEVerifier 生成 PKCE code_verifier（43–128 字符的高熵随机串）。
func NewPKCEVerifier() (string, error) {
	return oauth2.GenerateVerifier(), nil
}

// PKCEChallenge 由 code_verifier 计算 S256 挑战（code_challenge）。
func PKCEChallenge(verifier string) string {
	return oauth2.S256ChallengeFromVerifier(verifier)
}
