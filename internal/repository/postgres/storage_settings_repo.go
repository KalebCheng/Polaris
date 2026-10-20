package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const storageSettingsColumns = `id, type, endpoint, region, bucket, access_key, secret_key_enc,
	path_prefix, domain, root_path, public_base, path_style, updated_at`

// StorageSettingsRepository 系统级默认存储配置仓库（单行；PostgreSQL 实现）
type StorageSettingsRepository struct{ db *dbclient.Client }

// NewStorageSettingsRepository 创建系统存储配置仓库
func NewStorageSettingsRepository(db *dbclient.Client) *StorageSettingsRepository {
	return &StorageSettingsRepository{db: db}
}

// Get 读取单行配置；行不存在时返回空实体（不报错，便于首启无配置）。
func (r *StorageSettingsRepository) Get(ctx context.Context) (*repository.StorageSettings, error) {
	row := r.db.QueryRow(ctx, `SELECT `+storageSettingsColumns+` FROM storage_settings WHERE id=1`)
	var s repository.StorageSettings
	err := row.Scan(&s.ID, &s.Type, &s.Endpoint, &s.Region, &s.Bucket, &s.AccessKey, &s.SecretKeyEnc,
		&s.PathPrefix, &s.Domain, &s.RootPath, &s.PublicBase, &s.PathStyle, &s.UpdatedAt)
	if err != nil {
		if isNoRows(err) {
			return &repository.StorageSettings{ID: 1}, nil
		}
		return nil, mapError(err, "storage settings not found")
	}
	return &s, nil
}

// Update 更新单行配置（不存在则插入）。
func (r *StorageSettingsRepository) Update(ctx context.Context, s *repository.StorageSettings) error {
	const q = `INSERT INTO storage_settings (
		id, type, endpoint, region, bucket, access_key, secret_key_enc,
		path_prefix, domain, root_path, public_base, path_style, updated_at)
		VALUES (1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
		ON CONFLICT (id) DO UPDATE SET
			type=EXCLUDED.type, endpoint=EXCLUDED.endpoint, region=EXCLUDED.region,
			bucket=EXCLUDED.bucket, access_key=EXCLUDED.access_key, secret_key_enc=EXCLUDED.secret_key_enc,
			path_prefix=EXCLUDED.path_prefix, domain=EXCLUDED.domain, root_path=EXCLUDED.root_path,
			public_base=EXCLUDED.public_base, path_style=EXCLUDED.path_style, updated_at=NOW()`
	return mapError(r.db.Exec(ctx, q, s.Type, s.Endpoint, s.Region, s.Bucket, s.AccessKey, s.SecretKeyEnc,
		s.PathPrefix, s.Domain, s.RootPath, s.PublicBase, s.PathStyle), "")
}
