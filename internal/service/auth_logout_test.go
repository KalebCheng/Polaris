package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

type logoutSessionRepo struct {
	repository.SessionRepository
	deleted string
}

func (r *logoutSessionRepo) Get(_ context.Context, sessionID string) ([]byte, error) {
	session := auth.Session{
		ID:            sessionID,
		InstitutionID: 1,
		UserID:        2,
		Name:          "alice",
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	}
	return json.Marshal(session)
}

func (r *logoutSessionRepo) Delete(_ context.Context, sessionID string) error {
	r.deleted = sessionID
	return nil
}

func (r *logoutSessionRepo) RemoveUserSession(context.Context, int64, string, string) error {
	return nil
}

type logoutUserRepo struct {
	repository.UserRepository
	offlineID int64
}

func (r *logoutUserRepo) SetOnline(_ context.Context, id int64, online bool) error {
	if !online {
		r.offlineID = id
	}
	return nil
}

type logoutTokenRepo struct {
	repository.TokenRepository
	revokedSession string
}

func (r *logoutTokenRepo) RevokeBySession(_ context.Context, sessionID string) (int64, error) {
	r.revokedSession = sessionID
	return 1, nil
}

func TestLogoutRevokesSessionTokens(t *testing.T) {
	sessions := &logoutSessionRepo{}
	users := &logoutUserRepo{}
	tokens := &logoutTokenRepo{}
	svc := &AuthService{
		users:    users,
		sessions: auth.NewSessionManager(sessions, users, nil, nil, time.Hour),
		tokens:   tokens,
	}

	if err := svc.Logout(context.Background(), "session-1"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if tokens.revokedSession != "session-1" {
		t.Fatalf("revoked session = %q, want session-1", tokens.revokedSession)
	}
	if sessions.deleted != "session-1" {
		t.Fatalf("deleted session = %q, want session-1", sessions.deleted)
	}
	if users.offlineID != 2 {
		t.Fatalf("offline user = %d, want 2", users.offlineID)
	}
}
