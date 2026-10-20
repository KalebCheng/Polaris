package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"polaris/internal/auth/tokenhash"
	"polaris/internal/repository"
)

type tokenServiceTokenRepo struct {
	repository.TokenRepository
	created *repository.Token
}

func (r *tokenServiceTokenRepo) GetByName(context.Context, int64, string) (*repository.Token, error) {
	return nil, errors.New("not found")
}

func (r *tokenServiceTokenRepo) Create(_ context.Context, t *repository.Token) (int64, error) {
	r.created = t
	return 11, nil
}

type tokenServiceUserRepo struct {
	repository.UserRepository
}

func (tokenServiceUserRepo) GetByName(context.Context, int64, string) (*repository.User, error) {
	return &repository.User{ID: 5, InstitutionID: 1, Name: "alice"}, nil
}

func TestCreateAPIKeyPersistsHashAndReturnsPlaintextOnce(t *testing.T) {
	tokens := &tokenServiceTokenRepo{}
	svc := NewTokenService(tokens, tokenServiceUserRepo{}, nil)

	plain, rec, err := svc.CreateAPIKey(context.Background(), "admin", 1, "alice", "api-key", time.Hour)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if plain == "" {
		t.Fatal("plaintext API key must be returned exactly once")
	}
	if rec == nil || tokens.created == nil {
		t.Fatal("token record must be created")
	}
	if rec.AccessTokenHash != tokenhash.Hash(plain) || tokens.created.AccessTokenHash != tokenhash.Hash(plain) {
		t.Fatalf("stored access token hash does not match returned plaintext")
	}
}
