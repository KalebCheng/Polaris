package postgres

import (
	"context"
	"strings"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// ProviderTemplateRepository 消息模板仓库：邮件与短信是两张结构不同的子表。
type ProviderTemplateRepository struct{ db *dbclient.Client }

// NewProviderTemplateRepository 创建消息模板仓库。
func NewProviderTemplateRepository(db *dbclient.Client) *ProviderTemplateRepository {
	return &ProviderTemplateRepository{db: db}
}

const (
	providerEmailTemplateTable = "provider_email_templates"
	providerSMSTemplateTable   = "provider_sms_templates"
)

func (r *ProviderTemplateRepository) Create(ctx context.Context, t *repository.ProviderTemplate) (int64, error) {
	channel := normalizeTemplateChannel(t.Channel)
	if channel == "" {
		return 0, errInvalidTemplateChannel(t.Channel)
	}
	var id int64
	var createdAt, updatedAt time.Time
	var err error
	switch channel {
	case "email":
		err = r.db.QueryRow(ctx, `INSERT INTO provider_email_templates
			(institution_id, provider_id, name, scene, subject, content, is_default)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING id, created_at, updated_at`,
			t.InstitutionID, t.ProviderID, t.Name, t.Scene, t.Subject, t.Content, t.IsDefault).
			Scan(&id, &createdAt, &updatedAt)
	case "sms":
		err = r.db.QueryRow(ctx, `INSERT INTO provider_sms_templates
			(institution_id, provider_id, name, scene, template_code, template_params, is_default)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING id, created_at, updated_at`,
			t.InstitutionID, t.ProviderID, t.Name, t.Scene, t.TemplateCode, orEmpty(t.TemplateParams), t.IsDefault).
			Scan(&id, &createdAt, &updatedAt)
	}
	if err != nil {
		return 0, mapError(err, "provider template not found")
	}
	t.ID, t.Channel, t.CreatedAt, t.UpdatedAt = id, channel, createdAt, updatedAt
	return id, nil
}

func (r *ProviderTemplateRepository) GetByID(ctx context.Context, channel string, id int64) (*repository.ProviderTemplate, error) {
	channel = normalizeTemplateChannel(channel)
	if channel == "" {
		return nil, errInvalidTemplateChannel(channel)
	}
	t := &repository.ProviderTemplate{ID: id, Channel: channel}
	var err error
	switch channel {
	case "email":
		err = r.db.QueryRow(ctx, `SELECT institution_id, provider_id, name, scene, subject, content, is_default, created_at, updated_at
			FROM provider_email_templates WHERE id = $1`, id).
			Scan(&t.InstitutionID, &t.ProviderID, &t.Name, &t.Scene, &t.Subject, &t.Content,
				&t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
	case "sms":
		err = r.db.QueryRow(ctx, `SELECT institution_id, provider_id, name, scene, template_code, template_params, is_default, created_at, updated_at
			FROM provider_sms_templates WHERE id = $1`, id).
			Scan(&t.InstitutionID, &t.ProviderID, &t.Name, &t.Scene, &t.TemplateCode, &t.TemplateParams,
				&t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
	}
	if err != nil {
		return nil, mapError(err, "provider template not found")
	}
	return t, nil
}

func (r *ProviderTemplateRepository) ListByProvider(ctx context.Context, channel string, institutionID, providerID int64, scene string) ([]*repository.ProviderTemplate, error) {
	channel = normalizeTemplateChannel(channel)
	if channel == "" {
		return nil, errInvalidTemplateChannel(channel)
	}
	query := ""
	switch channel {
	case "email":
		query = `SELECT id, institution_id, provider_id, name, scene, subject, content, is_default, created_at, updated_at
			FROM provider_email_templates WHERE institution_id = $1 AND provider_id = $2`
	case "sms":
		query = `SELECT id, institution_id, provider_id, name, scene, template_code, template_params, is_default, created_at, updated_at
			FROM provider_sms_templates WHERE institution_id = $1 AND provider_id = $2`
	}
	args := []interface{}{institutionID, providerID}
	if scene != "" {
		args = append(args, scene)
		query += ` AND scene = $3`
	}
	query += ` ORDER BY is_default DESC, name ASC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	items := make([]*repository.ProviderTemplate, 0)
	for rows.Next() {
		t := &repository.ProviderTemplate{Channel: channel}
		if channel == "email" {
			err = rows.Scan(&t.ID, &t.InstitutionID, &t.ProviderID, &t.Name, &t.Scene, &t.Subject, &t.Content,
				&t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
		} else {
			err = rows.Scan(&t.ID, &t.InstitutionID, &t.ProviderID, &t.Name, &t.Scene, &t.TemplateCode, &t.TemplateParams,
				&t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
		}
		if err != nil {
			return nil, mapError(err, "")
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err, "")
	}
	return items, nil
}

func (r *ProviderTemplateRepository) Update(ctx context.Context, t *repository.ProviderTemplate) error {
	channel := normalizeTemplateChannel(t.Channel)
	if channel == "" {
		return errInvalidTemplateChannel(t.Channel)
	}
	var err error
	switch channel {
	case "email":
		err = r.db.Exec(ctx, `UPDATE provider_email_templates
			SET name=$2, scene=$3, subject=$4, content=$5, is_default=$6, updated_at=NOW()
			WHERE id=$1`, t.ID, t.Name, t.Scene, t.Subject, t.Content, t.IsDefault)
	case "sms":
		err = r.db.Exec(ctx, `UPDATE provider_sms_templates
			SET name=$2, scene=$3, template_code=$4, template_params=$5, is_default=$6, updated_at=NOW()
			WHERE id=$1`, t.ID, t.Name, t.Scene, t.TemplateCode, orEmpty(t.TemplateParams), t.IsDefault)
	}
	return mapError(err, "provider template not found")
}

func (r *ProviderTemplateRepository) Delete(ctx context.Context, channel string, id int64) error {
	channel = normalizeTemplateChannel(channel)
	if channel == "" {
		return errInvalidTemplateChannel(channel)
	}
	table := providerEmailTemplateTable
	if channel == "sms" {
		table = providerSMSTemplateTable
	}
	return mapError(r.db.Exec(ctx, `DELETE FROM `+table+` WHERE id = $1`, id), "provider template not found")
}

func normalizeTemplateChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "email":
		return "email"
	case "sms":
		return "sms"
	default:
		return ""
	}
}

func errInvalidTemplateChannel(channel string) error {
	return bizerrors.ErrInvalidParams.WithDetail("channel", channel)
}

var _ repository.ProviderTemplateRepository = (*ProviderTemplateRepository)(nil)
