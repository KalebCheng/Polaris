package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// SessionManager 会话管理器：创建/读取/销毁 Redis 会话，并计算有效权限。
type SessionManager struct {
	sessions   repository.SessionRepository
	users      repository.UserRepository
	roles      repository.RoleRepository
	groups     repository.GroupRepository
	sessionTTL time.Duration
}

// NewSessionManager 创建会话管理器
func NewSessionManager(
	sessions repository.SessionRepository,
	users repository.UserRepository,
	roles repository.RoleRepository,
	groups repository.GroupRepository,
	sessionTTL time.Duration,
) *SessionManager {
	return &SessionManager{sessions: sessions, users: users, roles: roles, groups: groups, sessionTTL: sessionTTL}
}

// EffectivePermissions 计算用户在组织内的有效权限规则。
func (m *SessionManager) EffectivePermissions(ctx context.Context, institutionID int64, user string) ([]PermissionRule, error) {
	perms, err := m.roles.PermissionsForUser(ctx, institutionID, user)
	if err != nil {
		return nil, err
	}
	rules := make([]PermissionRule, 0, len(perms))
	for _, p := range perms {
		for _, res := range p.Resources {
			for _, act := range p.Actions {
				rules = append(rules, PermissionRule{Name: p.Name, Resource: res, Action: act, Effect: defaultEffect(p.Effect)})
			}
		}
	}
	return rules, nil
}

// RolesForUser 返回用户在组织内的有效角色名（含经用户组/嵌套角色获得）。
func (m *SessionManager) RolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return m.roles.RolesForUser(ctx, institutionID, user)
}

func defaultEffect(effect string) string {
	if effect == "" {
		return "Allow"
	}
	return effect
}

// Create 创建会话并返回会话对象与 JWT 用的 session ID。
func (m *SessionManager) Create(ctx context.Context, user *repository.User, application, ip, userAgent string) (*Session, error) {
	roles, err := m.roles.RolesForUser(ctx, user.InstitutionID, user.Name)
	if err != nil {
		return nil, err
	}
	perms, err := m.EffectivePermissions(ctx, user.InstitutionID, user.Name)
	if err != nil {
		return nil, err
	}

	now := nowTime()
	expires := now.Add(m.sessionTTL)
	session := &Session{
		ID:            uuid.NewString(),
		UserID:        user.ID,
		InstitutionID: user.InstitutionID,
		Name:          user.Name,
		DisplayName:   user.DisplayName,
		Email:         user.Email,
		IsAdmin:       user.IsAdmin,
		Roles:         roles,
		Permissions:   perms,
		Application:   application,
		IP:            ip,
		UserAgent:     userAgent,
		CreatedAt:     now.Unix(),
		ExpiresAt:     expires.Unix(),
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "序列化会话失败", err)
	}
	if err := m.sessions.Create(ctx, session.ID, data, m.sessionTTL); err != nil {
		return nil, err
	}
	if err := m.sessions.AddUserSession(ctx, user.InstitutionID, user.Name, session.ID); err != nil {
		return nil, err
	}
	return session, nil
}

// SetMfaEnroll 设置会话的「待绑定 MFA」标记（组织强制 MFA 且用户未绑定时）。
func (m *SessionManager) SetMfaEnroll(ctx context.Context, sessionID string, required bool) error {
	session, err := m.Load(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.MfaEnrollRequired == required {
		return nil
	}
	session.MfaEnrollRequired = required
	ttl, err := m.sessions.TTL(ctx, sessionID)
	if err != nil || ttl <= 0 {
		ttl = m.sessionTTL
	}
	data, err := json.Marshal(session)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "序列化会话失败", err)
	}
	return m.sessions.Update(ctx, sessionID, data, ttl)
}

// Load 读取会话。
func (m *SessionManager) Load(ctx context.Context, sessionID string) (*Session, error) {
	data, err := m.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "解析会话失败", err)
	}
	if session.ExpiresAt > 0 && session.ExpiresAt < nowUnix() {
		_ = m.sessions.Delete(ctx, sessionID)
		return nil, bizerrors.ErrSessionExpired
	}
	return &session, nil
}

// Refresh 刷新会话内容与 TTL（例如重新计算权限后）。
func (m *SessionManager) Refresh(ctx context.Context, session *Session) error {
	perms, err := m.EffectivePermissions(ctx, session.InstitutionID, session.Name)
	if err != nil {
		return err
	}
	roles, err := m.roles.RolesForUser(ctx, session.InstitutionID, session.Name)
	if err != nil {
		return err
	}
	session.Permissions = perms
	session.Roles = roles
	session.ExpiresAt = nowTime().Add(m.sessionTTL).Unix()
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}
	return m.sessions.Update(ctx, session.ID, data, m.sessionTTL)
}

// Delete 删除单个会话。
func (m *SessionManager) Delete(ctx context.Context, session *Session) error {
	if err := m.sessions.Delete(ctx, session.ID); err != nil {
		return err
	}
	return m.sessions.RemoveUserSession(ctx, session.InstitutionID, session.Name, session.ID)
}

// Drop 删除指定会话并清理用户索引（供应用会话策略收敛使用）。
func (m *SessionManager) Drop(ctx context.Context, institutionID int64, user, sessionID string) error {
	if err := m.sessions.Delete(ctx, sessionID); err != nil {
		return err
	}
	return m.sessions.RemoveUserSession(ctx, institutionID, user, sessionID)
}

// InvalidateUser 失效用户全部会话（权限变更 / 禁用 / 改密）。
func (m *SessionManager) InvalidateUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return m.sessions.DeleteUserSessions(ctx, institutionID, user)
}

// UserSessions 返回用户当前会话 ID。
func (m *SessionManager) UserSessions(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return m.sessions.UserSessions(ctx, institutionID, user)
}

// AllSessions 返回全部（跨组织）活动会话，供平台级「会话」视图使用。
func (m *SessionManager) AllSessions(ctx context.Context) ([]*Session, error) {
	ids, err := m.sessions.ListSessionIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Session, 0, len(ids))
	for _, id := range ids {
		sess, err := m.Load(ctx, id)
		if err != nil {
			continue // 已过期
		}
		out = append(out, sess)
	}
	return out, nil
}

// TTL 返回会话剩余有效期。
func (m *SessionManager) TTL(ctx context.Context, sessionID string) (time.Duration, error) {
	return m.sessions.TTL(ctx, sessionID)
}
