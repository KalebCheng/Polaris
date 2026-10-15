package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const tokenColumns = `id, institution_id, application, "user", name, code, code_challenge,
	code_challenge_method, code_is_used, code_expire_in, code_scope, code_redirect_uri, access_token,
	refresh_token, id_token, access_token_hash, refresh_token_hash, id_token_hash, expires_in, scope,
	token_type, grant_type, resource, session_id, revoked, expire_time, created_at`

// TokenRepository 令牌仓库（PostgreSQL 实现）
type TokenRepository struct{ db *dbclient.Client }

// NewTokenRepository 创建令牌仓库
func NewTokenRepository(db *dbclient.Client) *TokenRepository { return &TokenRepository{db: db} }

func scanToken(row interface{ Scan(...interface{}) error }) (*repository.Token, error) {
	var t repository.Token
	err := row.Scan(&t.ID, &t.InstitutionID, &t.Application, &t.User, &t.Name, &t.Code, &t.CodeChallenge,
		&t.CodeChallengeMethod, &t.CodeIsUsed, &t.CodeExpireIn, &t.CodeScope, &t.CodeRedirectURI,
		&t.AccessToken, &t.RefreshToken, &t.IDToken, &t.AccessTokenHash, &t.RefreshTokenHash,
		&t.IDTokenHash, &t.ExpiresIn, &t.Scope, &t.TokenType, &t.GrantType, &t.Resource, &t.SessionID,
		&t.Revoked, &t.ExpireTime, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create 创建令牌记录
func (r *TokenRepository) Create(ctx context.Context, t *repository.Token) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO tokens (institution_id, application, "user", name, code,
		code_challenge, code_challenge_method, code_is_used, code_expire_in, code_scope, code_redirect_uri,
		access_token, refresh_token, id_token, access_token_hash, refresh_token_hash, id_token_hash,
		expires_in, scope, token_type, grant_type, resource, session_id, revoked, expire_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		RETURNING id`,
		t.InstitutionID, t.Application, t.User, t.Name, t.Code, t.CodeChallenge, t.CodeChallengeMethod,
		t.CodeIsUsed, t.CodeExpireIn, t.CodeScope, t.CodeRedirectURI, t.AccessToken, t.RefreshToken,
		t.IDToken, t.AccessTokenHash, t.RefreshTokenHash, t.IDTokenHash, t.ExpiresIn, t.Scope,
		defaultStr(t.TokenType, "Bearer"), t.GrantType, t.Resource, t.SessionID, t.Revoked, t.ExpireTime,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "token not found")
	}
	t.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *TokenRepository) GetByID(ctx context.Context, id int64) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenColumns+` FROM tokens WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByName 按组织+名称查询
func (r *TokenRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenColumns+` FROM tokens WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByCode 按授权码查询
func (r *TokenRepository) GetByCode(ctx context.Context, code string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenColumns+` FROM tokens WHERE code=$1`, code))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByAccessToken 按 access_token 查询
func (r *TokenRepository) GetByAccessToken(ctx context.Context, accessToken string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenColumns+` FROM tokens WHERE access_token=$1`, accessToken))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// GetByRefreshToken 按 refresh_token 查询
func (r *TokenRepository) GetByRefreshToken(ctx context.Context, refreshToken string) (*repository.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenColumns+` FROM tokens WHERE refresh_token=$1`, refreshToken))
	if err != nil {
		return nil, mapError(err, "token not found")
	}
	return t, nil
}

// ConsumeCode 原子标记授权码已使用（防止授权码重放）。
func (r *TokenRepository) ConsumeCode(ctx context.Context, code string) (bool, error) {
	tag, err := r.db.ExecTag(ctx, `UPDATE tokens SET code_is_used=TRUE WHERE code=$1 AND code_is_used=FALSE`, code)
	if err != nil {
		return false, mapError(err, "")
	}
	return tag.RowsAffected() > 0, nil
}

// Update 更新令牌记录
func (r *TokenRepository) Update(ctx context.Context, t *repository.Token) error {
	return mapError(r.db.Exec(ctx, `UPDATE tokens SET application=$2, "user"=$3, code=$4,
		code_challenge=$5, code_challenge_method=$6, code_is_used=$7, code_expire_in=$8, code_scope=$9,
		code_redirect_uri=$10, access_token=$11, refresh_token=$12, id_token=$13, access_token_hash=$14,
		refresh_token_hash=$15, id_token_hash=$16, expires_in=$17, scope=$18, token_type=$19,
		grant_type=$20, resource=$21, session_id=$22, revoked=$23, expire_time=$24 WHERE id=$1`,
		t.ID, t.Application, t.User, t.Code, t.CodeChallenge, t.CodeChallengeMethod, t.CodeIsUsed,
		t.CodeExpireIn, t.CodeScope, t.CodeRedirectURI, t.AccessToken, t.RefreshToken, t.IDToken,
		t.AccessTokenHash, t.RefreshTokenHash, t.IDTokenHash, t.ExpiresIn, t.Scope,
		defaultStr(t.TokenType, "Bearer"), t.GrantType, t.Resource, t.SessionID, t.Revoked, t.ExpireTime),
		"token not found")
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
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR "user" ILIKE ` + p + ` OR application ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + tokenColumns + ` FROM tokens` + b.where() + b.orderLimit("created_at DESC", q.Offset, q.Limit)
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
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR "user" ILIKE ` + p + ` OR application ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM tokens`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// ListExpired 扫描过期令牌（清理任务）
func (r *TokenRepository) ListExpired(ctx context.Context, before time.Time, limit int) ([]*repository.Token, error) {
	rows, err := r.db.Query(ctx, `SELECT `+tokenColumns+` FROM tokens
		WHERE (expire_time IS NOT NULL AND expire_time < $1) OR created_at < $1
		ORDER BY created_at ASC LIMIT $2`, before, limit)
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
