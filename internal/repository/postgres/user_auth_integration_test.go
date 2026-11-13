package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"polaris/internal/repository"
)

func TestUserAuthDetailsLoadFromChildTables(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "user-auth-details")

	var appID, syncerID, providerID int64
	if err := client.QueryRow(ctx, `INSERT INTO applications (institution_id, code, name)
		VALUES ($1, 'app-auth', 'Auth App') RETURNING id`, institutionID).Scan(&appID); err != nil {
		t.Fatalf("create application: %v", err)
	}
	if err := client.QueryRow(ctx, `INSERT INTO syncers (institution_id, name, type)
		VALUES ($1, 'sync-auth', 'dingtalk') RETURNING id`, institutionID).Scan(&syncerID); err != nil {
		t.Fatalf("create syncer: %v", err)
	}
	if err := client.QueryRow(ctx, `INSERT INTO providers (institution_id, name, display_name, category, type, state)
		VALUES ($1, 'totp-auth', 'TOTP Auth', 'MFA', 'totp', 'active') RETURNING id`, institutionID).Scan(&providerID); err != nil {
		t.Fatalf("create provider: %v", err)
	}

	repo := NewUserRepository(client)
	user := &repository.User{
		UID:                  fmt.Sprintf("uid-%d-alice", institutionID),
		InstitutionID:        institutionID,
		Name:                 "alice",
		DisplayName:          "Alice",
		Email:                "alice@example.test",
		PasswordHash:         "hash-1",
		PasswordType:         "bcrypt",
		PasswordSalt:         "pepper-1",
		MustChangePassword:   true,
		SignupApplication:    "app-auth",
		SignupApplicationID:  appID,
		RegisterType:         "syncer",
		RegisterSource:       "sync-auth",
		RegistrationSyncerID: syncerID,
		CreatedIP:            "10.0.0.1",
	}
	id, err := repo.Create(ctx, user)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if !user.HasPassword {
		t.Fatal("创建后应标记 has_password")
	}

	got, err := repo.GetByName(ctx, institutionID, "alice")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !got.HasPassword || got.PasswordHash != "hash-1" || got.PasswordType != "bcrypt" || got.PasswordSalt != "pepper-1" {
		t.Fatalf("密码子表未正确回填: %+v", got)
	}
	if !got.MustChangePassword {
		t.Fatal("导入/重置后的 must_change_password 未回填")
	}
	if got.SignupApplicationID != appID || got.SignupApplication != "app-auth" ||
		got.RegistrationSyncerID != syncerID || got.RegisterSource != "sync-auth" || got.CreatedIP != "10.0.0.1" {
		t.Fatalf("注册来源子表未正确回填: %+v", got)
	}
	if got.MfaEnabled {
		t.Fatal("尚未绑定 MFA 时 mfa_enabled 应为 false")
	}

	if err := client.Exec(ctx, `INSERT INTO user_mfa_credentials
		(institution_id, user_id, provider_id, type, status, secret_encrypted, recovery_codes_hashed, phone, metadata)
		VALUES ($1, $2, $3, 'totp', 'active', 'encrypted', '{}', '', '{}')`,
		institutionID, id, providerID); err != nil {
		t.Fatalf("create mfa credential: %v", err)
	}
	got, err = repo.GetByName(ctx, institutionID, "alice")
	if err != nil {
		t.Fatalf("get user with mfa: %v", err)
	}
	if !got.MfaEnabled {
		t.Fatal("存在 active credential 时 mfa_enabled 应为 true")
	}

	enabled, err := repo.List(ctx, repository.ListQuery{
		InstitutionID: institutionID,
		Filters:       []repository.ColumnFilter{{Field: "mfa_enabled", Value: true}},
	})
	if err != nil {
		t.Fatalf("filter mfa enabled: %v", err)
	}
	if len(enabled) != 1 || enabled[0].Name != "alice" {
		t.Fatalf("mfa 筛选结果错误: %+v", enabled)
	}
	disabled, err := repo.List(ctx, repository.ListQuery{
		InstitutionID: institutionID,
		Filters:       []repository.ColumnFilter{{Field: "mfa_enabled", Value: false}},
	})
	if err != nil {
		t.Fatalf("filter mfa disabled: %v", err)
	}
	if len(disabled) != 0 {
		t.Fatalf("mfa=false 不应命中已绑定用户: %+v", disabled)
	}

	beforeUpdate := got.UpdatedAt
	time.Sleep(2 * time.Millisecond)
	if err := repo.UpdateLastSignin(ctx, id, time.Now(), "10.0.0.2"); err != nil {
		t.Fatalf("update last signin: %v", err)
	}
	got, err = repo.GetByName(ctx, institutionID, "alice")
	if err != nil {
		t.Fatalf("get user after signin: %v", err)
	}
	if got.LastSigninIP != "10.0.0.2" || got.LastSigninTime == nil {
		t.Fatalf("登录统计未回填: %+v", got)
	}
	if !got.UpdatedAt.Equal(beforeUpdate) {
		t.Fatalf("登录更新不应修改 users.updated_at: before=%s after=%s", beforeUpdate, got.UpdatedAt)
	}

	got.PasswordHash = "hash-2"
	got.PasswordType = "salt"
	got.PasswordSalt = "pepper-2"
	got.MustChangePassword = false
	if err := repo.UpdatePassword(ctx, got, 10); err != nil {
		t.Fatalf("update password: %v", err)
	}
	history, err := repo.PasswordHistory(ctx, institutionID, "alice", 10)
	if err != nil {
		t.Fatalf("password history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("密码历史应有旧密码与新密码两条，实际 %+v", history)
	}
	if history[0].PasswordHash != "hash-2" || history[0].PasswordType != "salt" || history[0].PasswordSalt != "pepper-2" {
		t.Fatalf("密码历史未记录写入时算法/盐: %+v", history[0])
	}
	var activeCount, invalidatedCount int
	if err := client.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE invalidated_at IS NULL),
			count(*) FILTER (WHERE invalidated_at IS NOT NULL)
		FROM password_history WHERE user_id=$1`, id).Scan(&activeCount, &invalidatedCount); err != nil {
		t.Fatalf("count password history: %v", err)
	}
	if activeCount != 1 || invalidatedCount < 1 {
		t.Fatalf("密码历史状态错误: active=%d invalidated=%d", activeCount, invalidatedCount)
	}
	got, err = repo.GetByName(ctx, institutionID, "alice")
	if err != nil {
		t.Fatalf("get user after password change: %v", err)
	}
	if got.MustChangePassword {
		t.Fatal("改密成功后必须清除 must_change_password")
	}

	got.PasswordHash = "hash-3"
	got.MustChangePassword = false
	if err := repo.UpdatePassword(ctx, got, 0); err != nil {
		t.Fatalf("update password with history disabled: %v", err)
	}
	if err := client.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE invalidated_at IS NULL),
			count(*) FILTER (WHERE invalidated_at IS NOT NULL)
		FROM password_history WHERE user_id=$1`, id).Scan(&activeCount, &invalidatedCount); err != nil {
		t.Fatalf("count password history after prune: %v", err)
	}
	if activeCount != 1 || invalidatedCount != 0 {
		t.Fatalf("history_count=0 应只保留 active 行: active=%d invalidated=%d", activeCount, invalidatedCount)
	}

	createdBySyncer, err := repo.ListByRegistrationSyncer(ctx, institutionID, syncerID, 10)
	if err != nil {
		t.Fatalf("list by registration syncer: %v", err)
	}
	if len(createdBySyncer) != 1 || createdBySyncer[0].ID != id {
		t.Fatalf("连接来源反查失败: %+v", createdBySyncer)
	}
}

func TestUsersTableNoLongerStoresAuthDetails(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	var count int
	if err := client.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='users' AND column_name IN (
			'password_hash','password_salt','password_type','password_updated_at',
			'last_signin_time','last_signin_ip','is_online',
			'mfa_enabled','mfa_type','mfa_secret','mfa_recovery_codes',
			'signup_application','register_type','register_source','created_ip','score'
		)`).Scan(&count); err != nil {
		t.Fatalf("query users columns: %v", err)
	}
	if count != 0 {
		t.Fatalf("users 仍包含已拆出的列: %d", count)
	}
}
