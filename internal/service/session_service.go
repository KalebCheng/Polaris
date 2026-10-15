package service

import (
	"context"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
)

// SessionService 会话服务（管理员视图：查看与强制下线）
type SessionService struct {
	sessions *auth.SessionManager
	audits   *AuditRecorder
}

// NewSessionService 创建会话服务
func NewSessionService(sessions *auth.SessionManager, audits *AuditRecorder) *SessionService {
	return &SessionService{sessions: sessions, audits: audits}
}

// UserSessions 返回用户当前会话明细。
func (s *SessionService) UserSessions(ctx context.Context, institutionID int64, user string) ([]*auth.Session, error) {
	ids, err := s.sessions.UserSessions(ctx, institutionID, user)
	if err != nil {
		return nil, err
	}
	out := make([]*auth.Session, 0, len(ids))
	for _, id := range ids {
		sess, err := s.sessions.Load(ctx, id)
		if err != nil {
			continue // 已过期
		}
		out = append(out, sess)
	}
	return out, nil
}

// AllSessions 返回平台级（跨组织）活动会话明细。
func (s *SessionService) AllSessions(ctx context.Context) ([]*auth.Session, error) {
	return s.sessions.AllSessions(ctx)
}

// OwnSessions 返回当前用户自己的会话。
func (s *SessionService) OwnSessions(ctx context.Context, institutionID int64, user string) ([]*auth.Session, error) {
	items, err := s.UserSessions(ctx, institutionID, user)
	if err != nil {
		return nil, err
	}
	owned := make([]*auth.Session, 0, len(items))
	for _, session := range items {
		if session.InstitutionID == institutionID && session.Name == user {
			owned = append(owned, session)
		}
	}
	return owned, nil
}

// RevokeOwnSession 只撤销确实归属于当前用户的会话。
func (s *SessionService) RevokeOwnSession(ctx context.Context, institutionID int64, user, sessionID string) error {
	sess, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.InstitutionID != institutionID || sess.Name != user {
		return bizerrors.ErrNotFound
	}
	if err := s.sessions.Delete(ctx, sess); err != nil {
		return err
	}
	s.audits.Record(ctx, user, institutionID, "session.self_revoke", user, map[string]interface{}{"session_id": sessionID})
	return nil
}

// RevokeOwnSessions 撤销当前用户的全部会话。
func (s *SessionService) RevokeOwnSessions(ctx context.Context, institutionID int64, user string) error {
	ids, err := s.sessions.InvalidateUser(ctx, institutionID, user)
	if err != nil {
		return err
	}
	s.audits.Record(ctx, user, institutionID, "session.self_revoke_all", user, map[string]interface{}{"count": len(ids)})
	return nil
}

// Revoke 强制下线指定会话。
func (s *SessionService) Revoke(ctx context.Context, actor string, institutionID int64, user, sessionID string) error {
	sess, err := s.sessions.Load(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := s.sessions.Delete(ctx, sess); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "session.revoke", user, map[string]interface{}{"session_id": sessionID})
	return nil
}

// RevokeAll 强制下线用户全部会话。
func (s *SessionService) RevokeAll(ctx context.Context, actor string, institutionID int64, user string) error {
	ids, err := s.sessions.InvalidateUser(ctx, institutionID, user)
	if err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "session.revoke_all", user, map[string]interface{}{"count": len(ids)})
	return nil
}
