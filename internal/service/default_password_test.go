package service

import (
	"testing"

	"polaris/internal/repository"
)

func TestProtectDefaultPasswordEncryptsAndPreserves(t *testing.T) {
	cipher := testCipher(t)
	svc := &InstitutionService{cipher: cipher}

	org := &repository.Institution{DefaultPasswordPlain: "default-secret"}
	if err := svc.protectDefaultPassword(org, ""); err != nil {
		t.Fatalf("protectDefaultPassword: %v", err)
	}
	if org.DefaultPasswordEnc == "" || org.DefaultPasswordPlain != "" {
		t.Fatalf("default password was not encrypted in place: %+v", org)
	}
	plain, err := cipher.Decrypt(org.DefaultPasswordEnc)
	if err != nil || plain != "default-secret" {
		t.Fatalf("decrypted default password = %q, %v", plain, err)
	}

	preserved := &repository.Institution{}
	if err := svc.protectDefaultPassword(preserved, "existing-ciphertext"); err != nil {
		t.Fatalf("preserveDefaultPassword: %v", err)
	}
	if preserved.DefaultPasswordEnc != "existing-ciphertext" {
		t.Fatalf("existing ciphertext was not preserved: %+v", preserved)
	}
}

func TestUserServiceDefaultPasswordDecrypts(t *testing.T) {
	cipher := testCipher(t)
	encrypted, err := cipher.Encrypt("default-secret")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	svc := &UserService{cipher: cipher}
	plain, err := svc.defaultPassword(&repository.Institution{DefaultPasswordEnc: encrypted})
	if err != nil {
		t.Fatalf("defaultPassword: %v", err)
	}
	if plain != "default-secret" {
		t.Fatalf("defaultPassword = %q, want default-secret", plain)
	}
}
