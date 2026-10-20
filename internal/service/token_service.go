package service

import (
	"context"
	"time"

	"polaris/internal/auth/tokenhash"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// TokenService 令牌服务（授权记录 / API 令牌的内省与吊销）
type TokenService struct {
	tokens repository.TokenRepository
	users  repository.UserRepository
	audits *AuditRecorder
}

// NewTokenService 创建令牌服务
func NewTokenService(tokens repository.TokenRepository, users repository.UserRepository, audits *AuditRecorder) *TokenService {
	return &TokenService{tokens: tokens, users: users, audits: audits}
}

// List 分页列出令牌
func (s *TokenService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Token, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.tokens.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.tokens.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get 查询令牌
func (s *TokenService) Get(ctx context.Context, institutionID int64, name string) (*repository.Token, error) {
	t, err := s.tokens.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeTokenNotFound, "token not found").WithDetail("token", name)
	}
	return t, nil
}

// Revoke 吊销令牌
func (s *TokenService) Revoke(ctx context.Context, actor string, institutionID int64, name string) error {
	t, err := s.tokens.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeTokenNotFound, "token not found").WithDetail("token", name)
	}
	if err := s.tokens.Revoke(ctx, t.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "token.revoke", name, nil)
	return nil
}

// Delete 删除令牌记录
func (s *TokenService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	t, err := s.tokens.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.New(bizerrors.CodeTokenNotFound, "token not found").WithDetail("token", name)
	}
	if err := s.tokens.Delete(ctx, t.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "token.delete", name, nil)
	return nil
}

// CreateAPIKey 为用户签发 API 访问令牌（长期有效，用于程序化调用）。
// 返回的明文令牌只在本次响应中出现，数据库只保存哈希。
func (s *TokenService) CreateAPIKey(ctx context.Context, actor string, institutionID int64, user, name string, ttl time.Duration) (string, *repository.Token, error) {
	if name == "" {
		name = "api-" + randomHex(6)
	}
	if _, err := s.tokens.GetByName(ctx, institutionID, name); err == nil {
		return "", nil, bizerrors.New(bizerrors.CodeConflict, "token name already exists")
	}
	userRecord, err := s.users.GetByName(ctx, institutionID, user)
	if err != nil {
		return "", nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	token := randomHex(32)
	expire := time.Now().Add(ttl)
	rec := &repository.Token{
		InstitutionID:   institutionID,
		UserID:          &userRecord.ID,
		Name:            name,
		AccessTokenHash: tokenhash.Hash(token),
		TokenType:       "Bearer",
		GrantType:       "api_key",
		Scope:           "api",
		ExpiresIn:       int(ttl.Seconds()),
		ExpireTime:      &expire,
	}
	id, err := s.tokens.Create(ctx, rec)
	if err != nil {
		return "", nil, err
	}
	rec.ID = id
	s.audits.Record(ctx, actor, institutionID, "token.create_api_key", name, nil)
	return token, rec, nil
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *TokenService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.tokens.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
