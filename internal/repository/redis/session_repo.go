// Package redis 提供基于 Redis 的仓库实现（会话 / 限流 / 临时态）。
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	bizerrors "polaris/internal/errors"
	"polaris/pkg/redisclient"
)

// SessionKeyPrefix 会话键前缀
const SessionKeyPrefix = "polaris:session:"

// userSessionsKey 用户会话索引键（Set）
func userSessionsKey(institutionID int64, user string) string {
	return fmt.Sprintf("polaris:user_sessions:%d:%s", institutionID, user)
}

// SessionRepository 会话仓库（Redis 实现）
type SessionRepository struct{ client *redisclient.Client }

// NewSessionRepository 创建会话仓库
func NewSessionRepository(client *redisclient.Client) *SessionRepository {
	return &SessionRepository{client: client}
}

// Create 创建会话
func (r *SessionRepository) Create(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error {
	if err := r.client.Set(ctx, SessionKeyPrefix+sessionID, data, ttl); err != nil {
		return bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return nil
}

// Get 读取会话
func (r *SessionRepository) Get(ctx context.Context, sessionID string) ([]byte, error) {
	v, err := r.client.Get(ctx, SessionKeyPrefix+sessionID)
	if err != nil {
		if err == redis.Nil {
			return nil, bizerrors.ErrSessionNotFound
		}
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return []byte(v), nil
}

// Update 刷新会话（保持 TTL）
func (r *SessionRepository) Update(ctx context.Context, sessionID string, data []byte, ttl time.Duration) error {
	return r.Create(ctx, sessionID, data, ttl)
}

// Delete 删除会话
func (r *SessionRepository) Delete(ctx context.Context, sessionID string) error {
	if err := r.client.Del(ctx, SessionKeyPrefix+sessionID); err != nil {
		return bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return nil
}

// Exists 判断会话是否存在
func (r *SessionRepository) Exists(ctx context.Context, sessionID string) (bool, error) {
	n, err := r.client.Exists(ctx, SessionKeyPrefix+sessionID)
	if err != nil {
		return false, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return n > 0, nil
}

// TTL 返回会话剩余有效期
func (r *SessionRepository) TTL(ctx context.Context, sessionID string) (time.Duration, error) {
	return r.client.TTL(ctx, SessionKeyPrefix+sessionID)
}

// AddUserSession 维护用户会话索引
func (r *SessionRepository) AddUserSession(ctx context.Context, institutionID int64, user, sessionID string) error {
	return r.client.SAdd(ctx, userSessionsKey(institutionID, user), sessionID)
}

// RemoveUserSession 移除用户会话索引
func (r *SessionRepository) RemoveUserSession(ctx context.Context, institutionID int64, user, sessionID string) error {
	return r.client.SRem(ctx, userSessionsKey(institutionID, user), sessionID)
}

// UserSessions 返回用户当前会话 ID 列表
func (r *SessionRepository) UserSessions(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return r.client.SMembers(ctx, userSessionsKey(institutionID, user))
}

// DeleteUserSessions 删除用户全部会话并清理索引
func (r *SessionRepository) DeleteUserSessions(ctx context.Context, institutionID int64, user string) ([]string, error) {
	ids, err := r.UserSessions(ctx, institutionID, user)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, SessionKeyPrefix+id)
	}
	if len(keys) > 0 {
		if err := r.client.Del(ctx, keys...); err != nil {
			return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
		}
	}
	if err := r.client.Del(ctx, userSessionsKey(institutionID, user)); err != nil {
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	return ids, nil
}

// ListSessionIDs 返回全部会话 ID（跨组织，供平台级「会话」视图使用）。
// 直接扫描会话键前缀，避免依赖用户会话索引的完备性。
func (r *SessionRepository) ListSessionIDs(ctx context.Context) ([]string, error) {
	keys, err := r.client.Scan(ctx, SessionKeyPrefix+"*", 500)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeRedisError, "redis error").WithError(err)
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, strings.TrimPrefix(key, SessionKeyPrefix))
	}
	return ids, nil
}
