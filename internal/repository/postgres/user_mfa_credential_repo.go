package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// UserMFACredentialRepository 用户 MFA 凭证仓库（PostgreSQL）。
type UserMFACredentialRepository struct{ db *dbclient.Client }

// NewUserMFACredentialRepository 创建用户 MFA 凭证仓库。
func NewUserMFACredentialRepository(db *dbclient.Client) *UserMFACredentialRepository {
	return &UserMFACredentialRepository{db: db}
}

const userMFACredentialTable = "user_mfa_credentials"

func (r *UserMFACredentialRepository) Get(ctx context.Context, institutionID, userID, providerID int64) (*repository.UserMFACredential, error) {
	item := &repository.UserMFACredential{}
	if err := selectOne(ctx, r.db, userMFACredentialTable, item,
		"WHERE institution_id = $1 AND user_id = $2 AND provider_id = $3", institutionID, userID, providerID); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *UserMFACredentialRepository) ListByUser(ctx context.Context, institutionID, userID int64) ([]*repository.UserMFACredential, error) {
	return queryEntities[repository.UserMFACredential](ctx, r.db, userMFACredentialTable,
		"WHERE institution_id = $1 AND user_id = $2", []interface{}{institutionID, userID}, "created_at ASC, id ASC", 0, 0)
}

// Save 按 (user_id, provider_id) 幂等写入：重新绑定同一 provider 时覆盖旧凭证。
func (r *UserMFACredentialRepository) Save(ctx context.Context, c *repository.UserMFACredential) (int64, error) {
	existing, err := r.Get(ctx, c.InstitutionID, c.UserID, c.ProviderID)
	if err == nil {
		c.ID = existing.ID
		c.CreatedAt = existing.CreatedAt
		if err := r.Update(ctx, c); err != nil {
			return 0, err
		}
		return c.ID, nil
	}
	id, err := insertReturningID(ctx, r.db, userMFACredentialTable, c)
	if err != nil {
		return 0, err
	}
	c.ID = id
	return id, nil
}

func (r *UserMFACredentialRepository) Update(ctx context.Context, c *repository.UserMFACredential) error {
	return updateEntity(ctx, r.db, userMFACredentialTable, c, "id")
}

func (r *UserMFACredentialRepository) Delete(ctx context.Context, institutionID, userID, providerID int64) error {
	err := r.db.Exec(ctx, `DELETE FROM user_mfa_credentials WHERE institution_id = $1 AND user_id = $2 AND provider_id = $3`, institutionID, userID, providerID)
	return mapError(err, "mfa credential not found")
}

var _ repository.UserMFACredentialRepository = (*UserMFACredentialRepository)(nil)
