package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris/internal/auth/tokenhash"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const tokenColumns = `t.id, t.institution_id, t.application_id, t.user_id, t.name, t.code_hash, t.code_challenge,
	t.code_challenge_method, t.code_is_used, t.code_expire_in, t.code_scope, t.code_redirect_uri, t.code_nonce,
	t.access_token_hash, t.refresh_token_hash, t.id_token_hash, t.expires_in, t.scope,
	t.token_type, t.grant_type, t.resource, t.session_id, t.revoked, t.expire_time, t.created_at`

// tokenSelect：读取时把主键解析回展示名（应用 code / 用户账号名）。
// 存储只认主键（§3.8），但对外契约保持 `application` / `user` 名称不变——前端与既有客户端零改动。
const tokenSelect = `SELECT ` + tokenColumns + `,
	COALESCE((SELECT a.code FROM applications a WHERE a.id = t.application_id), '') AS application,
	COALESCE((SELECT u.name FROM users u WHERE u.id = t.user_id), '') AS "user"
	FROM tokens t`

// TokenRepository 令牌仓库（PostgreSQL 实现）
type TokenRepository struct{ db *dbclient.Client }

// NewTokenRepository 创建令牌仓库
func NewTokenRepository(db *dbclient.Client) *TokenRepository { return &TokenRepository{db: db} }

func scanToken(row interface{ Scan(...interface{}) error }) (*repository.Token, error) {
	var t repository.Token
	err := row.Scan(&t.ID, &t.InstitutionID, &t.ApplicationID, &t.UserID, &t.Name, &t.CodeHash, &t.CodeChallenge,
		&t.CodeChallengeMethod, &t.CodeIsUsed, &t.CodeExpireIn, &t.CodeScope, &t.CodeRedirectURI,
		&t.CodeNonce, &t.AccessTokenHash, &t.RefreshTokenHash, &t.IDTokenHash, &t.ExpiresIn, &t.Scope,
		&t.TokenType, &t.GrantType, &t.Resource, &t.SessionID,
		&t.Revoked, &t.ExpireTime, &t.CreatedAt, &t.Application, &t.User)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

type tokenRowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func insertToken(ctx context.Context, q tokenRowQuerier, t *repository.Token) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO tokens (institution_id, application_id, user_id, name, code_hash,
		code_challenge, code_challenge_method, code_is_used, code_expire_in, code_scope, code_redirect_uri, code_nonce,
		access_token_hash, refresh_token_hash, id_token_hash,
		expires_in, scope, token_type, grant_type, resource, session_id, revoked, expire_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		RETURNING id`,
		t.InstitutionID, t.ApplicationID, t.UserID, t.Name, t.CodeHash, t.CodeChallenge, t.CodeChallengeMethod,
		t.CodeIsUsed, t.CodeExpireIn, t.CodeScope, t.CodeRedirectURI, t.CodeNonce, t.AccessTokenHash,
		t.RefreshTokenHash, t.IDTokenHash, t.ExpiresIn, t.Scope,
		defaultStr(t.TokenType, "Bearer"), t.GrantType, t.Resource, t.SessionID, t.Revoked, t.ExpireTime,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "token not found")
	}
	t.ID = id
	return id, nil
}

// Create inserts a token record.
func (r *TokenRepository) Create(ctx context.Context, t *repository.Token) (int64, error) {
	return insertToken(ctx, r.db, t)
}

// GetByID 按 ID 查询
func (r *TokenRepository) GetByID(ctx context.Context, id int64) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, tokenSelect+` WHERE t.id=$1`, id))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByName 按组织+名称查询
func (r *TokenRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, tokenSelect+` WHERE t.institution_id=$1 AND t.name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByCode 按应用 + 授权码哈希查询。
func (r *TokenRepository) GetByCode(ctx context.Context, applicationID int64, code string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, tokenSelect+` WHERE t.application_id=$1 AND t.code_hash=$2`, applicationID, tokenhash.Hash(code)))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByAccessToken 按 access_token 查询
func (r *TokenRepository) GetByAccessToken(ctx context.Context, accessToken string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, tokenSelect+` WHERE t.access_token_hash=$1`, tokenhash.Hash(accessToken)))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByRefreshToken 按应用 + refresh_token 哈希查询。
func (r *TokenRepository) GetByRefreshToken(ctx context.Context, applicationID int64, refreshToken string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, tokenSelect+` WHERE t.application_id=$1 AND t.refresh_token_hash=$2`, applicationID, tokenhash.Hash(refreshToken)))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// ConsumeCode 原子标记授权码已使用（防止授权码重放）。
func (r *TokenRepository) ConsumeCode(ctx context.Context, applicationID int64, code string) (bool, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE tokens SET code_is_used=TRUE
		WHERE application_id=$1 AND code_hash=$2 AND code_is_used=FALSE`, applicationID, tokenhash.Hash(code))
	if err != nil {
		return false, mapError(err, "")
	}
	return tag.RowsAffected() > 0, nil
}

// Update 更新令牌记录
func (r *TokenRepository) Update(ctx context.Context, t *repository.Token) error {
	return mapError(r.db.Exec(ctx, `UPDATE tokens SET application_id=$2, user_id=$3, code_hash=$4,
		code_challenge=$5, code_challenge_method=$6, code_is_used=$7, code_expire_in=$8, code_scope=$9,
		code_redirect_uri=$10, code_nonce=$11, access_token_hash=$12, refresh_token_hash=$13, id_token_hash=$14,
		expires_in=$15, scope=$16, token_type=$17, grant_type=$18, resource=$19, session_id=$20,
		revoked=$21, expire_time=$22 WHERE id=$1`,
		t.ID, t.ApplicationID, t.UserID, t.CodeHash, t.CodeChallenge, t.CodeChallengeMethod, t.CodeIsUsed,
		t.CodeExpireIn, t.CodeScope, t.CodeRedirectURI, t.CodeNonce, t.AccessTokenHash, t.RefreshTokenHash,
		t.IDTokenHash, t.ExpiresIn, t.Scope, defaultStr(t.TokenType, "Bearer"), t.GrantType, t.Resource,
		t.SessionID, t.Revoked, t.ExpireTime),
		"token not found")
}

// RotateRefreshToken revokes oldID and stores replacement atomically.
func (r *TokenRepository) RotateRefreshToken(ctx context.Context, oldID int64, replacement *repository.Token) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tokens
			SET revoked=TRUE
			WHERE id=$1 AND revoked=FALSE`, oldID)
		if err != nil {
			return mapError(err, "")
		}
		if tag.RowsAffected() != 1 {
			return bizerrors.ErrOAuthInvalidGrant.WithDetail("reason", "refresh_invalid")
		}
		_, err = insertToken(ctx, tx, replacement)
		return err
	})
}

// RevokeBySession revokes every active token bound to a Polaris session.
func (r *TokenRepository) RevokeBySession(ctx context.Context, sessionID string) (int64, error) {
	if sessionID == "" {
		return 0, nil
	}
	tag, err := r.db.ExecTag(ctx, `UPDATE tokens SET revoked=TRUE
		WHERE session_id=$1 AND revoked=FALSE`, sessionID)
	if err != nil {
		return 0, mapError(err, "")
	}
	return tag.RowsAffected(), nil
}

// Revoke 吊销令牌
func (r *TokenRepository) Revoke(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `UPDATE tokens SET revoked=TRUE WHERE id=$1`, id), "token not found")
}

// Delete 删除令牌记录
func (r *TokenRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM tokens WHERE id=$1`, id), "token not found")
}

// List 分页列出令牌
func (r *TokenRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Token, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		// 过滤字段允许 application_id / user_id（主键）；名称过滤走关键词
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.name ILIKE ` + p + ` OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	sql := tokenSelect + b.where() + b.orderLimit("t.created_at DESC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, t)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计令牌数量
func (r *TokenRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.name ILIKE ` + p + ` OR EXISTS (SELECT 1 FROM users u WHERE u.id = t.user_id AND u.name ILIKE ` + p + `)
			OR EXISTS (SELECT 1 FROM applications a WHERE a.id = t.application_id AND a.code ILIKE ` + p + `))`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM tokens t`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// ListExpired 扫描过期令牌（清理任务）
func (r *TokenRepository) ListExpired(ctx context.Context, before time.Time, limit int) ([]*repository.Token, error) {
	rows, err := r.db.Query(ctx, tokenSelect+`
		WHERE (t.expire_time IS NOT NULL AND t.expire_time < $1) OR t.created_at < $1
		ORDER BY t.created_at ASC LIMIT $2`, before, limit)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Token
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, t)
	}
	return result, mapError(rows.Err(), "")
}

// DeleteBatch 批量删除令牌
func (r *TokenRepository) DeleteBatch(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return mapError(r.db.Exec(ctx, `DELETE FROM tokens WHERE id = ANY($1)`, ids), "")
}

var _ repository.TokenRepository = (*TokenRepository)(nil)
