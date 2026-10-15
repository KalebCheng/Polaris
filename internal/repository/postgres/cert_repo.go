package postgres

import (
	"context"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const certColumns = `id, institution_id, name, display_name, scope, type, crypto_algorithm, bit_size,
	expire_in_years, certificate, private_key, expire_time, is_system, created_at, updated_at`

// CertRepository 证书仓库（PostgreSQL 实现）
type CertRepository struct{ db *dbclient.Client }

// NewCertRepository 创建证书仓库
func NewCertRepository(db *dbclient.Client) *CertRepository { return &CertRepository{db: db} }

func scanCert(row interface{ Scan(...interface{}) error }) (*repository.Cert, error) {
	var c repository.Cert
	err := row.Scan(&c.ID, &c.InstitutionID, &c.Name, &c.DisplayName, &c.Scope, &c.Type, &c.CryptoAlgorithm,
		&c.BitSize, &c.ExpireInYears, &c.Certificate, &c.PrivateKey, &c.ExpireTime, &c.IsSystem,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Create 创建证书
func (r *CertRepository) Create(ctx context.Context, c *repository.Cert) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO certs (institution_id, name, display_name, scope, type,
		crypto_algorithm, bit_size, expire_in_years, certificate, private_key, expire_time, is_system)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		c.InstitutionID, c.Name, c.DisplayName, defaultStr(c.Scope, "JWT"), defaultStr(c.Type, "x509"),
		defaultStr(c.CryptoAlgorithm, "RSA"), c.BitSize, c.ExpireInYears, c.Certificate, c.PrivateKey,
		c.ExpireTime, c.IsSystem).Scan(&id)
	if err != nil {
		return 0, mapError(err, "certificate not found")
	}
	c.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *CertRepository) GetByID(ctx context.Context, id int64) (*repository.Cert, error) {
	c, err := scanCert(r.db.QueryRow(ctx, `SELECT `+certColumns+` FROM certs WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "certificate not found")
	}
	return c, nil
}

// GetByName 按组织+名称查询
func (r *CertRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Cert, error) {
	c, err := scanCert(r.db.QueryRow(ctx, `SELECT `+certColumns+` FROM certs WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "certificate not found")
	}
	return c, nil
}

// Update 更新证书
func (r *CertRepository) Update(ctx context.Context, c *repository.Cert) error {
	return mapError(r.db.Exec(ctx, `UPDATE certs SET display_name=$2, scope=$3, type=$4,
		crypto_algorithm=$5, bit_size=$6, expire_in_years=$7, certificate=$8, private_key=$9,
		expire_time=$10, updated_at=NOW() WHERE id=$1`,
		c.ID, c.DisplayName, defaultStr(c.Scope, "JWT"), defaultStr(c.Type, "x509"),
		defaultStr(c.CryptoAlgorithm, "RSA"), c.BitSize, c.ExpireInYears, c.Certificate, c.PrivateKey,
		c.ExpireTime), "certificate not found")
}

// Delete 删除证书
func (r *CertRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM certs WHERE id=$1`, id), "certificate not found")
}

// List 分页列出证书
func (r *CertRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Cert, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + certColumns + ` FROM certs` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Cert
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, c)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计证书数量
func (r *CertRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM certs`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

var _ repository.CertRepository = (*CertRepository)(nil)
