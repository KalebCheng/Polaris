package repository

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSensitiveEntityFieldsNeverMarshal(t *testing.T) {
	cases := []struct {
		value any
		keys  []string
	}{
		{&User{PasswordHash: "hash", PasswordSalt: "salt", MfaSecret: "totp", MfaRecoveryCodes: []string{"code"}},
			[]string{"password_hash", "password_salt", "mfa_secret", "mfa_recovery_codes"}},
		{&Application{ClientSecretHash: "application-secret-hash", ClientSecretPlain: "application-secret"},
			[]string{"client_secret_hash", "client_secret_plain"}},
		{&Institution{DefaultPasswordEnc: "default-password-cipher", DefaultPasswordPlain: "default-password"},
			[]string{"default_password_enc", "default_password_plain"}},
		{&Provider{ClientSecret: "provider-secret"}, []string{"client_secret"}},
		{&Cert{PrivateKey: "private-key"}, []string{"private_key"}},
		{&Token{CodeHash: "code-hash", AccessTokenHash: "access-hash", RefreshTokenHash: "refresh-hash", IDTokenHash: "id-hash"},
			[]string{"code_hash", "access_token_hash", "refresh_token_hash", "id_token_hash"}},
		{&PasswordHistory{PasswordHash: "history-hash"}, []string{"password_hash"}},
	}
	for _, tc := range cases {
		value := tc.value
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %T: %v", value, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal %T: %v", value, err)
		}
		for _, key := range tc.keys {
			if _, ok := decoded[key]; ok {
				t.Fatalf("%T leaked sensitive field %q: %s", value, key, raw)
			}
		}
	}
}

func TestInstitutionDefaultPasswordPlainIsNotPersisted(t *testing.T) {
	field, ok := reflect.TypeOf(Institution{}).FieldByName("DefaultPasswordPlain")
	if !ok {
		t.Fatal("Institution.DefaultPasswordPlain is missing")
	}
	if got := field.Tag.Get("db"); got != "-" {
		t.Fatalf("DefaultPasswordPlain db tag = %q, want -", got)
	}
	if got := field.Tag.Get("json"); got != "-" {
		t.Fatalf("DefaultPasswordPlain json tag = %q, want -", got)
	}
}

func TestApplicationClientSecretPlainIsNotPersisted(t *testing.T) {
	field, ok := reflect.TypeOf(Application{}).FieldByName("ClientSecretPlain")
	if !ok {
		t.Fatal("Application.ClientSecretPlain is missing")
	}
	if got := field.Tag.Get("db"); got != "-" {
		t.Fatalf("ClientSecretPlain db tag = %q, want -", got)
	}
	if got := field.Tag.Get("json"); got != "-" {
		t.Fatalf("ClientSecretPlain json tag = %q, want -", got)
	}
}
