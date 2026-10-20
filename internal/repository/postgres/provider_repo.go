package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// ProviderRepository 提供商仓库（PostgreSQL）。
//
// 存储采用主表 + 类别子表：
//   - providers 只存公共身份与状态；
//   - provider_<category> 存类型专属配置；
//   - 仓库对外仍返回聚合后的 repository.Provider，调用方无需感知表结构。
type ProviderRepository struct{ db *dbclient.Client }

// NewProviderRepository 创建提供商仓库。
func NewProviderRepository(db *dbclient.Client) *ProviderRepository {
	return &ProviderRepository{db: db}
}

const providerTable = "providers"

func (r *ProviderRepository) Create(ctx context.Context, p *repository.Provider) (int64, error) {
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = insertReturningID(ctx, tx, providerTable, p)
		if err != nil {
			return err
		}
		p.ID = id
		return upsertProviderConfig(ctx, tx, p)
	})
	if err != nil {
		return 0, err
	}
	p.ID = id
	return id, nil
}

func (r *ProviderRepository) GetByID(ctx context.Context, id int64) (*repository.Provider, error) {
	p := &repository.Provider{}
	if err := selectOne(ctx, r.db, providerTable, p, "WHERE id = $1", id); err != nil {
		return nil, err
	}
	if err := loadProviderConfig(ctx, r.db, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProviderRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Provider, error) {
	p := &repository.Provider{}
	if err := selectOne(ctx, r.db, providerTable, p, "WHERE institution_id = $1 AND name = $2", institutionID, name); err != nil {
		return nil, err
	}
	if err := loadProviderConfig(ctx, r.db, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProviderRepository) Update(ctx context.Context, p *repository.Provider) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		const q = `UPDATE providers
		              SET display_name = $2, category = $3, type = $4, state = $5, updated_at = NOW()
		            WHERE id = $1`
		if err := mapError(execTx(ctx, tx, q, p.ID, p.DisplayName, p.Category, p.Type, p.State), "provider not found"); err != nil {
			return err
		}
		return upsertProviderConfig(ctx, tx, p)
	})
}

func (r *ProviderRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM providers WHERE id = $1`, id), "provider not found")
}

func (r *ProviderRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Provider, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + ` = ` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		pattern := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + pattern + ` OR display_name ILIKE ` + pattern + `)`)
	}
	items, err := queryEntities[repository.Provider](ctx, r.db, providerTable, b.where(), b.args, "category ASC, name ASC", q.Offset, q.Limit)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := loadProviderConfig(ctx, r.db, item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// ListIdentityLinkProviders 返回绑定到该入方向连接、启用身份链接匹配的提供商。
//
// "同源身份连接"反向认领使用：同步侧据此知道该连接上有哪些提供商需要回写/反查外部身份。
func (r *ProviderRepository) ListIdentityLinkProviders(ctx context.Context, institutionID, syncerID int64) ([]*repository.Provider, error) {
	if institutionID == 0 || syncerID <= 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `SELECT p.id FROM providers p
		JOIN provider_oauth o ON o.provider_id = p.id
		WHERE p.institution_id = $1 AND o.match_syncer_id = $2
		  AND o.match_type = 'identity_link' AND p.state = 'active'
		ORDER BY p.id ASC`, institutionID, syncerID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	ids := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err, "")
	}
	out := make([]*repository.Provider, 0, len(ids))
	for _, id := range ids {
		item, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *ProviderRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id = ` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(q.Field + ` = ` + b.arg(q.Value))
	}
	if q.Keyword != "" {
		pattern := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + pattern + ` OR display_name ILIKE ` + pattern + `)`)
	}
	return countEntities(ctx, r.db, providerTable, b.where(), b.args...)
}

func loadProviderConfig(ctx context.Context, db *dbclient.Client, p *repository.Provider) error {
	switch strings.ToLower(strings.TrimSpace(p.Category)) {
	case "oauth":
		err := db.QueryRow(ctx, `SELECT client_id, client_secret, custom_auth_url, custom_token_url,
			custom_user_info_url, issuer_url, scopes, enable_pkce, auto_create,
			match_type, match_local_field, match_provider_field, COALESCE(match_syncer_id, 0),
			match_syncer_field, link_by_email, allowed_domains, redirect_uri,
			provision_username_field, provision_name_field, provision_email_field,
			provision_phone_field, provision_avatar_field
			  FROM provider_oauth WHERE provider_id = $1`, p.ID).
			Scan(&p.ClientID, &p.ClientSecret, &p.CustomAuthURL, &p.CustomTokenURL,
				&p.CustomUserInfoURL, &p.IssuerURL, &p.Scopes, &p.EnablePKCE, &p.AutoCreate,
				&p.MatchType, &p.MatchLocalField, &p.MatchProviderField, &p.MatchSyncerID,
				&p.MatchSyncerField, &p.LinkByEmail, &p.AllowedDomains, &p.RedirectURI,
				&p.ProvisionUsernameField, &p.ProvisionNameField, &p.ProvisionEmailField,
				&p.ProvisionPhoneField, &p.ProvisionAvatarField)
		return mapError(err, "provider oauth config not found")
	case "email":
		err := db.QueryRow(ctx, `SELECT host, port, ssl_mode, from_address, client_id, client_secret, receiver
			  FROM provider_email WHERE provider_id = $1`, p.ID).
			Scan(&p.Host, &p.Port, &p.SslMode, &p.FromAddress, &p.ClientID, &p.ClientSecret, &p.Receiver)
		return mapError(err, "provider email config not found")
	case "sms":
		err := db.QueryRow(ctx, `SELECT region_id, sign_name, client_id, client_secret, endpoint, receiver
			  FROM provider_sms WHERE provider_id = $1`, p.ID).
			Scan(&p.RegionID, &p.SignName, &p.ClientID, &p.ClientSecret, &p.Endpoint, &p.Receiver)
		return mapError(err, "provider sms config not found")
	case "captcha":
		err := db.QueryRow(ctx, `SELECT client_id, client_secret FROM provider_captcha WHERE provider_id = $1`, p.ID).
			Scan(&p.ClientID, &p.ClientSecret)
		return mapError(err, "provider captcha config not found")
	case "storage":
		err := db.QueryRow(ctx, `SELECT endpoint, domain, bucket, path_prefix, client_id, client_secret
			  FROM provider_storage WHERE provider_id = $1`, p.ID).
			Scan(&p.Endpoint, &p.Domain, &p.Bucket, &p.PathPrefix, &p.ClientID, &p.ClientSecret)
		return mapError(err, "provider storage config not found")
	case "mfa":
		err := db.QueryRow(ctx, `SELECT mfa_issuer, mfa_digits, mfa_period, mfa_algorithm,
			COALESCE(mfa_sms_provider_id, 0), COALESCE(mfa_sms_template_id, 0),
			mfa_code_ttl_seconds, mfa_remember_hours
			  FROM provider_mfa WHERE provider_id = $1`, p.ID).
			Scan(&p.MfaIssuer, &p.MfaDigits, &p.MfaPeriod, &p.MfaAlgorithm,
				&p.MfaSMSProviderID, &p.MfaSMSTemplateID, &p.MfaCodeTTLSeconds, &p.MfaRememberHours)
		return mapError(err, "provider mfa config not found")
	default:
		return nil
	}
}

func upsertProviderConfig(ctx context.Context, tx pgx.Tx, p *repository.Provider) error {
	switch strings.ToLower(strings.TrimSpace(p.Category)) {
	case "oauth":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_oauth (
			provider_id, institution_id, client_id, client_secret, custom_auth_url, custom_token_url,
			custom_user_info_url, issuer_url, scopes, enable_pkce, auto_create,
			match_type, match_local_field, match_provider_field, match_syncer_id, match_syncer_field,
			link_by_email, allowed_domains, redirect_uri,
			provision_username_field, provision_name_field, provision_email_field,
			provision_phone_field, provision_avatar_field
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,0),$16,$17,$18,$19,$20,$21,$22,$23,$24)
		ON CONFLICT (provider_id) DO UPDATE SET
			client_id=EXCLUDED.client_id, client_secret=EXCLUDED.client_secret,
			custom_auth_url=EXCLUDED.custom_auth_url, custom_token_url=EXCLUDED.custom_token_url,
			custom_user_info_url=EXCLUDED.custom_user_info_url, issuer_url=EXCLUDED.issuer_url,
			scopes=EXCLUDED.scopes, enable_pkce=EXCLUDED.enable_pkce, auto_create=EXCLUDED.auto_create,
			match_type=EXCLUDED.match_type, match_local_field=EXCLUDED.match_local_field,
			match_provider_field=EXCLUDED.match_provider_field, match_syncer_id=EXCLUDED.match_syncer_id,
			match_syncer_field=EXCLUDED.match_syncer_field,
			link_by_email=EXCLUDED.link_by_email, allowed_domains=EXCLUDED.allowed_domains,
			redirect_uri=EXCLUDED.redirect_uri,
			provision_username_field=EXCLUDED.provision_username_field,
			provision_name_field=EXCLUDED.provision_name_field,
			provision_email_field=EXCLUDED.provision_email_field,
			provision_phone_field=EXCLUDED.provision_phone_field,
			provision_avatar_field=EXCLUDED.provision_avatar_field`,
			p.ID, p.InstitutionID, p.ClientID, p.ClientSecret, p.CustomAuthURL, p.CustomTokenURL,
			p.CustomUserInfoURL, p.IssuerURL, orEmpty(p.Scopes), p.EnablePKCE, p.AutoCreate,
			p.MatchType, p.MatchLocalField, p.MatchProviderField, p.MatchSyncerID, p.MatchSyncerField,
			p.LinkByEmail, orEmpty(p.AllowedDomains), p.RedirectURI,
			p.ProvisionUsernameField, p.ProvisionNameField, p.ProvisionEmailField,
			p.ProvisionPhoneField, p.ProvisionAvatarField), "")
	case "email":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_email (
			provider_id, institution_id, host, port, ssl_mode, from_address, client_id, client_secret, receiver
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (provider_id) DO UPDATE SET
			host=EXCLUDED.host, port=EXCLUDED.port, ssl_mode=EXCLUDED.ssl_mode,
			from_address=EXCLUDED.from_address, client_id=EXCLUDED.client_id,
			client_secret=EXCLUDED.client_secret, receiver=EXCLUDED.receiver`,
			p.ID, p.InstitutionID, p.Host, p.Port, p.SslMode, p.FromAddress, p.ClientID, p.ClientSecret, p.Receiver), "")
	case "sms":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_sms (
			provider_id, institution_id, region_id, sign_name, client_id, client_secret, endpoint, receiver
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (provider_id) DO UPDATE SET
			region_id=EXCLUDED.region_id, sign_name=EXCLUDED.sign_name, client_id=EXCLUDED.client_id,
			client_secret=EXCLUDED.client_secret, endpoint=EXCLUDED.endpoint, receiver=EXCLUDED.receiver`,
			p.ID, p.InstitutionID, p.RegionID, p.SignName, p.ClientID, p.ClientSecret, p.Endpoint, p.Receiver), "")
	case "captcha":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_captcha (
			provider_id, institution_id, client_id, client_secret
		) VALUES ($1,$2,$3,$4)
		ON CONFLICT (provider_id) DO UPDATE SET
			client_id=EXCLUDED.client_id, client_secret=EXCLUDED.client_secret`,
			p.ID, p.InstitutionID, p.ClientID, p.ClientSecret), "")
	case "storage":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_storage (
			provider_id, institution_id, endpoint, domain, bucket, path_prefix, client_id, client_secret
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (provider_id) DO UPDATE SET
			endpoint=EXCLUDED.endpoint, domain=EXCLUDED.domain, bucket=EXCLUDED.bucket,
			path_prefix=EXCLUDED.path_prefix, client_id=EXCLUDED.client_id,
			client_secret=EXCLUDED.client_secret`,
			p.ID, p.InstitutionID, p.Endpoint, p.Domain, p.Bucket, p.PathPrefix, p.ClientID, p.ClientSecret), "")
	case "mfa":
		return mapError(execTx(ctx, tx, `INSERT INTO provider_mfa (
			provider_id, institution_id, mfa_issuer, mfa_digits, mfa_period, mfa_algorithm,
			mfa_sms_provider_id, mfa_sms_template_id, mfa_code_ttl_seconds, mfa_remember_hours
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (provider_id) DO UPDATE SET
			mfa_issuer=EXCLUDED.mfa_issuer, mfa_digits=EXCLUDED.mfa_digits,
			mfa_period=EXCLUDED.mfa_period, mfa_algorithm=EXCLUDED.mfa_algorithm,
			mfa_sms_provider_id=EXCLUDED.mfa_sms_provider_id,
			mfa_sms_template_id=EXCLUDED.mfa_sms_template_id,
			mfa_code_ttl_seconds=EXCLUDED.mfa_code_ttl_seconds,
			mfa_remember_hours=EXCLUDED.mfa_remember_hours`,
			p.ID, p.InstitutionID, p.MfaIssuer, p.MfaDigits, p.MfaPeriod, p.MfaAlgorithm,
			nullableID(p.MfaSMSProviderID), nullableID(p.MfaSMSTemplateID),
			p.MfaCodeTTLSeconds, p.MfaRememberHours), "")
	default:
		return nil
	}
}

func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}

func execTx(ctx context.Context, tx pgx.Tx, sql string, args ...interface{}) error {
	_, err := tx.Exec(ctx, sql, args...)
	return err
}

var _ repository.ProviderRepository = (*ProviderRepository)(nil)
