package postgres

import (
	"context"
	"strconv"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// instColumns 租户表列清单（顺序与 scanInstitution 一致）。
const instColumns = `id, name, display_name, domain, website_url, logo, favicon, description, captcha,
	password_type, password_salt, password_options, password_expire_days, password_history_count,
	token_retention_days, record_retention_days, default_application, default_password,
	use_email_as_username, disable_signin,
	mfa_policy, mfa_remember_hours, status, created_at, updated_at`

// InstitutionRepository 租户仓库（PostgreSQL 实现）
type InstitutionRepository struct {
	db *dbclient.Client
}

// NewInstitutionRepository 创建租户仓库
func NewInstitutionRepository(db *dbclient.Client) *InstitutionRepository {
	return &InstitutionRepository{db: db}
}

func scanInstitution(row interface{ Scan(...interface{}) error }) (*repository.Institution, error) {
	var o repository.Institution
	err := row.Scan(&o.ID, &o.Name, &o.DisplayName, &o.Domain, &o.WebsiteURL, &o.Logo, &o.Favicon,
		&o.Description, &o.Captcha,
		&o.PasswordType, &o.PasswordSalt, &o.PasswordOptions, &o.PasswordExpireDays, &o.PasswordHistoryCount,
		&o.TokenRetentionDays, &o.RecordRetentionDays, &o.DefaultApplication, &o.DefaultPassword,
		&o.UseEmailAsUsername, &o.DisableSignin,
		&o.MfaPolicy, &o.MfaRememberHours, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// Create 创建租户
func (r *InstitutionRepository) Create(ctx context.Context, inst *repository.Institution) (int64, error) {
	const q = `INSERT INTO institutions (
		name, display_name, domain, website_url, logo, favicon, description, captcha,
		password_type, password_salt, password_options, password_expire_days, password_history_count,
		token_retention_days, record_retention_days, default_application, default_password,
		use_email_as_username, disable_signin,
		mfa_policy, mfa_remember_hours, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
		        COALESCE(NULLIF($20,''),'optional'), $21, COALESCE(NULLIF($22,''),'active'))
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, q,
		inst.Name, inst.DisplayName, inst.Domain, inst.WebsiteURL, inst.Logo, inst.Favicon,
		inst.Description, inst.Captcha,
		defaultStr(inst.PasswordType, "bcrypt"), inst.PasswordSalt, orEmpty(inst.PasswordOptions),
		inst.PasswordExpireDays, inst.PasswordHistoryCount, inst.TokenRetentionDays, inst.RecordRetentionDays,
		inst.DefaultApplication, inst.DefaultPassword, inst.UseEmailAsUsername, inst.DisableSignin,
		inst.MfaPolicy, inst.MfaRememberHours, inst.Status,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "institution not found")
	}
	inst.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *InstitutionRepository) GetByID(ctx context.Context, id int64) (*repository.Institution, error) {
	o, err := scanInstitution(r.db.QueryRow(ctx, `SELECT `+instColumns+` FROM institutions WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "institution not found")
	}
	return o, nil
}

// GetByName 按名称查询
func (r *InstitutionRepository) GetByName(ctx context.Context, name string) (*repository.Institution, error) {
	o, err := scanInstitution(r.db.QueryRow(ctx, `SELECT `+instColumns+` FROM institutions WHERE name=$1`, name))
	if err != nil {
		return nil, mapError(err, "institution not found")
	}
	return o, nil
}

// GetByDomain 按域名查询（Host 解析租户；大小写不敏感，空域名不匹配）
func (r *InstitutionRepository) GetByDomain(ctx context.Context, domain string) (*repository.Institution, error) {
	o, err := scanInstitution(r.db.QueryRow(ctx,
		`SELECT `+instColumns+` FROM institutions WHERE domain <> '' AND lower(domain)=lower($1)`, domain))
	if err != nil {
		return nil, mapError(err, "institution not found")
	}
	return o, nil
}

// Update 更新租户
func (r *InstitutionRepository) Update(ctx context.Context, inst *repository.Institution) error {
	const q = `UPDATE institutions SET
		display_name=$2, domain=$3, website_url=$4, logo=$5, favicon=$6, description=$7, captcha=$8,
		password_type=$9, password_salt=$10, password_options=$11, password_expire_days=$12, password_history_count=$13,
		token_retention_days=$14, record_retention_days=$15, default_application=$16,
		default_password=$17, use_email_as_username=$18, disable_signin=$19,
		mfa_policy=$20, mfa_remember_hours=$21, status=$22, updated_at=NOW()
		WHERE id=$1`
	err := r.db.Exec(ctx, q, inst.ID, inst.DisplayName, inst.Domain, inst.WebsiteURL, inst.Logo,
		inst.Favicon, inst.Description, inst.Captcha, defaultStr(inst.PasswordType, "bcrypt"),
		inst.PasswordSalt, orEmpty(inst.PasswordOptions), inst.PasswordExpireDays, inst.PasswordHistoryCount,
		inst.TokenRetentionDays, inst.RecordRetentionDays, inst.DefaultApplication, inst.DefaultPassword,
		inst.UseEmailAsUsername, inst.DisableSignin,
		defaultStr(inst.MfaPolicy, repository.MfaPolicyOptional), inst.MfaRememberHours, defaultStr(inst.Status, "active"))
	return mapError(err, "institution not found")
}

// Delete 删除租户
func (r *InstitutionRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM institutions WHERE id=$1`, id), "institution not found")
}

// List 分页列出组织
func (r *InstitutionRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Institution, error) {
	sql := `SELECT ` + instColumns + ` FROM institutions WHERE 1=1`
	args := []interface{}{}
	if q.Keyword != "" {
		args = append(args, "%"+q.Keyword+"%")
		p := "$" + strconv.Itoa(len(args))
		sql += ` AND (name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`
	}
	sql += ` ORDER BY name ASC`
	if q.Limit > 0 {
		args = append(args, q.Limit, q.Offset)
		sql += ` LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
	}
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()

	var result []*repository.Institution
	for rows.Next() {
		o, err := scanInstitution(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, o)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计租户数量
func (r *InstitutionRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	sql := `SELECT COUNT(*) FROM institutions WHERE 1=1`
	args := []interface{}{}
	if q.Keyword != "" {
		args = append(args, "%"+q.Keyword+"%")
		sql += ` AND (name ILIKE $1 OR display_name ILIKE $1)`
	}
	var n int64
	err := r.db.QueryRow(ctx, sql, args...).Scan(&n)
	return n, mapError(err, "")
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
