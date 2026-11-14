package postgres

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris/internal/repository"
	"polaris/pkg/migrator"
)

// rollbackFrom 回滚到指定版本之前，避免测试写死“只回滚 1 步”而被后续迁移破坏。
func rollbackFrom(t *testing.T, pool *pgxpool.Pool, dir string, version int64) {
	t.Helper()
	migrations, err := migrator.LoadDir(dir)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	steps := 0
	for _, item := range migrations {
		if item.Version >= version {
			steps++
		}
	}
	if _, err := migrator.Down(context.Background(), pool, dir, steps); err != nil {
		t.Fatalf("rollback to before %d: %v", version, err)
	}
}

// 回归：006 必须把存量 text[] 平台变量名转换为同名 JSONB 映射，并且 down 能还原。
func TestProviderTemplateParamMappingMigration(t *testing.T) {
	client, pool := integrationClient(t)
	ctx := context.Background()
	dir := filepath.Join("..", "..", "..", "migrations")
	rollbackFrom(t, pool, dir, 6)

	var columnType string
	if err := pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns
		WHERE table_name='provider_sms_templates' AND column_name='template_params'`).Scan(&columnType); err != nil {
		t.Fatalf("inspect legacy column: %v", err)
	}
	if columnType != "ARRAY" {
		t.Fatalf("template_params column type = %s, want ARRAY before 006", columnType)
	}

	institutionID := createInstitutionFixture(t, client, "provider-template-migration")
	providers := NewProviderRepository(client)
	providerID, err := providers.Create(ctx, &repository.Provider{
		InstitutionID: institutionID,
		Name:          "sms-legacy",
		DisplayName:   "SMS legacy",
		Category:      "sms",
		Type:          "aliyun",
		ClientID:      "access-key",
		ClientSecret:  "access-secret",
		SignName:      "Polaris",
		State:         "disabled",
	})
	if err != nil {
		t.Fatalf("create sms provider: %v", err)
	}
	var templateID int64
	if err := pool.QueryRow(ctx, `INSERT INTO provider_sms_templates
		(institution_id, provider_id, name, scene, template_code, template_params, is_default)
		VALUES ($1,$2,'legacy-login','login_code','SMS_1',ARRAY['code','minutes'],true)
		RETURNING id`, institutionID, providerID).Scan(&templateID); err != nil {
		t.Fatalf("insert legacy template: %v", err)
	}

	if _, err := migrator.Up(ctx, pool, dir); err != nil {
		t.Fatalf("apply 006: %v", err)
	}
	pool.Reset()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT template_params FROM provider_sms_templates WHERE id=$1`, templateID).Scan(&raw); err != nil {
		t.Fatalf("load migrated params: %v", err)
	}
	var params []repository.ProviderTemplateParam
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatalf("decode migrated params: %v (raw=%s)", err, raw)
	}
	if len(params) != 2 ||
		params[0] != (repository.ProviderTemplateParam{Key: "code", Source: "code"}) ||
		params[1] != (repository.ProviderTemplateParam{Key: "minutes", Source: "minutes"}) {
		t.Fatalf("migrated params = %+v", params)
	}

	rollbackFrom(t, pool, dir, 6)
	pool.Reset()
	var legacy []string
	if err := pool.QueryRow(ctx, `SELECT template_params FROM provider_sms_templates WHERE id=$1`, templateID).Scan(&legacy); err != nil {
		t.Fatalf("load rolled-back params: %v", err)
	}
	if len(legacy) != 2 || legacy[0] != "code" || legacy[1] != "minutes" {
		t.Fatalf("legacy params after down = %+v", legacy)
	}
}
