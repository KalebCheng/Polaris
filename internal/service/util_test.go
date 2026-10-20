package service

import (
	"errors"
	"testing"
)

func TestRandomCredentialsFailClosed(t *testing.T) {
	original := secureRandomRead
	t.Cleanup(func() { secureRandomRead = original })
	secureRandomRead = func([]byte) (int, error) { return 0, errors.New("random source failed") }

	defer func() {
		if recover() == nil {
			t.Fatal("randomHex must not fall back to predictable data")
		}
	}()
	_ = randomHex(32)
}
