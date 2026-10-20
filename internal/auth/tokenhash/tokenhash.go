// Package tokenhash provides the canonical hash used for opaque token storage
// and lookup. Tokens are high-entropy random values, so a single SHA-256
// digest is sufficient to prevent offline recovery from the stored value.
package tokenhash

import (
	"crypto/sha256"
	"encoding/base64"
)

// Hash returns the base64url-encoded SHA-256 digest of token.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
