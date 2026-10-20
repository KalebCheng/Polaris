package service

import (
	"testing"

	"polaris/internal/auth"
)

func testCipher(t *testing.T) *auth.SecretCipher {
	t.Helper()
	cipher, err := auth.NewSecretCipher(auth.DeriveKey("test-credential-key"))
	if err != nil {
		t.Fatalf("NewSecretCipher: %v", err)
	}
	return cipher
}
