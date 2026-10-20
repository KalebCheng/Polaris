package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris/internal/auth/tokenhash"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
	"polaris/pkg/migrator"
)

func integrationClient(t *testing.T) (*dbclient.Client, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("POLARIS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set POLARIS_TEST_DATABASE_URL to a disposable PostgreSQL *_test database")
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database DSN: %v", err)
	}
	if !strings.HasSuffix(poolConfig.ConnConfig.Database, "_test") {
		t.Fatalf("refusing to reset non-test database %q", poolConfig.ConnConfig.Database)
	}

	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop test schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	if _, err := migrator.Up(ctx, pool, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	client, err := dbclient.New(dbclient.Config{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 1})
	if err != nil {
		t.Fatalf("create repository client: %v", err)
	}
	t.Cleanup(client.Close)
	return client, pool
}

func TestMigrationBaselineSupportsDownAndUp(t *testing.T) {
	_, pool := integrationClient(t)
	ctx := context.Background()
	dir := filepath.Join("..", "..", "..", "migrations")

	down, err := migrator.Down(ctx, pool, dir, 1)
	if err != nil || len(down) != 1 {
		t.Fatalf("down = %v, err = %v", down, err)
	}
	var usersTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.users')::text`).Scan(&usersTable); err != nil {
		t.Fatalf("check users table: %v", err)
	}
	if usersTable != nil {
		t.Fatalf("users table still exists after down: %s", *usersTable)
	}
	up, err := migrator.Up(ctx, pool, dir)
	if err != nil || len(up) != 1 {
		t.Fatalf("up = %v, err = %v", up, err)
	}
}

func createInstitutionFixture(t *testing.T, client *dbclient.Client, name string) int64 {
	t.Helper()
	repo := NewInstitutionRepository(client)
	item := &repository.Institution{Name: name, DisplayName: name}
	id, err := repo.Create(context.Background(), item)
	if err != nil {
		t.Fatalf("create institution %q: %v", name, err)
	}
	return id
}

func createUserFixture(t *testing.T, client *dbclient.Client, institutionID int64, name string) int64 {
	t.Helper()
	repo := NewUserRepository(client)
	item := &repository.User{
		UID:           fmt.Sprintf("uid-%d-%s", institutionID, name),
		InstitutionID: institutionID,
		Name:          name,
		DisplayName:   name,
		Email:         name + "@example.test",
	}
	id, err := repo.Create(context.Background(), item)
	if err != nil {
		t.Fatalf("create user %q: %v", name, err)
	}
	return id
}

func TestApplicationSubjectsUsePrimaryKeysAndCascade(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "application-subjects")
	userID := createUserFixture(t, client, institutionID, "alice")

	jobTitles := NewJobTitleRepository(client)
	jobTitle := &repository.JobTitle{
		InstitutionID: institutionID,
		Name:          "operator",
		DisplayName:   "Operator",
		IsEnabled:     true,
	}
	if _, err := jobTitles.Create(ctx, jobTitle); err != nil {
		t.Fatalf("create job title: %v", err)
	}

	departments := NewDepartmentRepository(client)
	department := &repository.Department{
		InstitutionID: institutionID,
		Name:          "operations",
		DisplayName:   "Operations",
		IsRoot:        true,
	}
	if _, err := departments.Create(ctx, department); err != nil {
		t.Fatalf("create department: %v", err)
	}

	apps := NewApplicationRepository(client)
	app := &repository.Application{
		InstitutionID: institutionID,
		Code:          "console",
		Name:          "Console",
		State:         "active",
		AccessMode:    repository.AccessModeRestricted,
	}
	if _, err := apps.Create(ctx, app); err != nil {
		t.Fatalf("create application: %v", err)
	}

	subjects := NewApplicationSubjectRepository(client)
	if err := subjects.Replace(ctx, app, repository.AccessModeRestricted, []repository.ApplicationSubject{
		{SubjectType: repository.SubjectTypeUser, UserID: &userID},
		{SubjectType: repository.SubjectTypeJobTitle, JobTitleID: &jobTitle.ID},
		{SubjectType: repository.SubjectTypeDepartment, DepartmentID: &department.ID},
	}); err != nil {
		t.Fatalf("replace application subjects: %v", err)
	}

	items, err := subjects.List(ctx, app.ID)
	if err != nil {
		t.Fatalf("list application subjects: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("subject count = %d, want 3", len(items))
	}
	byType := map[string]repository.ApplicationSubject{}
	for _, item := range items {
		byType[item.SubjectType] = item
	}
	if byType[repository.SubjectTypeUser].UserID == nil || *byType[repository.SubjectTypeUser].UserID != userID {
		t.Fatalf("user subject did not retain user_id: %+v", byType[repository.SubjectTypeUser])
	}
	if byType[repository.SubjectTypeJobTitle].JobTitleID == nil || *byType[repository.SubjectTypeJobTitle].JobTitleID != jobTitle.ID {
		t.Fatalf("job-title subject did not retain job_title_id: %+v", byType[repository.SubjectTypeJobTitle])
	}
	if byType[repository.SubjectTypeDepartment].DepartmentID == nil || *byType[repository.SubjectTypeDepartment].DepartmentID != department.ID {
		t.Fatalf("department subject did not retain department_id: %+v", byType[repository.SubjectTypeDepartment])
	}

	if err := client.Exec(ctx, `INSERT INTO application_subjects
		(institution_id, application_id, subject_type, user_id, job_title_id)
		VALUES ($1, $2, 'user', $3, $4)`,
		institutionID, app.ID, userID, jobTitle.ID); err == nil ||
		!strings.Contains(err.Error(), "application_subjects_subject_id_check") {
		t.Fatalf("mismatched subject columns must violate the CHECK constraint, got %v", err)
	}

	if err := client.Exec(ctx, `DELETE FROM departments WHERE id=$1`, department.ID); err != nil {
		t.Fatalf("delete department: %v", err)
	}
	var remaining int
	if err := client.QueryRow(ctx, `SELECT COUNT(*) FROM application_subjects WHERE application_id=$1`, app.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining subjects: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("department subject was not cascaded, remaining = %d", remaining)
	}
}

func TestProviderOAuthIdentityLinkMatchSchema(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()

	var columnCount int
	if err := client.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='provider_oauth'
		  AND column_name IN ('match_type', 'match_syncer_id', 'match_syncer_field')`).Scan(&columnCount); err != nil {
		t.Fatalf("check provider_oauth match columns: %v", err)
	}
	if columnCount != 3 {
		t.Fatalf("provider_oauth match column count = %d, want 3", columnCount)
	}

	var provisionCount int
	if err := client.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='provider_oauth'
		  AND column_name IN ('provision_username_field', 'provision_name_field', 'provision_email_field',
		                      'provision_phone_field', 'provision_avatar_field')`).Scan(&provisionCount); err != nil {
		t.Fatalf("check provider_oauth provision columns: %v", err)
	}
	if provisionCount != 5 {
		t.Fatalf("provider_oauth provision column count = %d, want 5", provisionCount)
	}

	var attributesColumn bool
	if err := client.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM information_schema.columns
		WHERE table_schema='public' AND table_name='user_identities' AND column_name='attributes'
	)`).Scan(&attributesColumn); err != nil {
		t.Fatalf("check user_identities.attributes: %v", err)
	}
	if !attributesColumn {
		t.Fatal("user_identities.attributes must exist for same-source reverse correlation")
	}

	var scopedFK bool
	if err := client.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM pg_constraint c
		JOIN pg_class t ON t.oid=c.conrelid
		JOIN pg_class r ON r.oid=c.confrelid
		WHERE t.relname='provider_oauth' AND r.relname='syncers'
		  AND c.contype='f' AND c.confdeltype='r'
	)`).Scan(&scopedFK); err != nil {
		t.Fatalf("check provider_oauth syncer FK: %v", err)
	}
	if !scopedFK {
		t.Fatal("provider_oauth.match_syncer_id must reference syncers with ON DELETE RESTRICT")
	}
}

func TestIdentityLinkMatchCandidatesAreScopedToSyncer(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "identity-match-scope")
	firstUserID := createUserFixture(t, client, institutionID, "first")
	secondUserID := createUserFixture(t, client, institutionID, "second")

	var firstSyncerID, secondSyncerID int64
	if err := client.QueryRow(ctx, `INSERT INTO syncers (name, type, institution_id)
		VALUES ('source-a', 'http', $1) RETURNING id`, institutionID).Scan(&firstSyncerID); err != nil {
		t.Fatalf("create first syncer: %v", err)
	}
	if err := client.QueryRow(ctx, `INSERT INTO syncers (name, type, institution_id)
		VALUES ('source-b', 'http', $1) RETURNING id`, institutionID).Scan(&secondSyncerID); err != nil {
		t.Fatalf("create second syncer: %v", err)
	}
	if err := client.Exec(ctx, `INSERT INTO syncer_user_links
		(institution_id, syncer_id, user_id, external_id) VALUES
		($1, $2, $3, 'same-external-id'), ($1, $4, $5, 'same-external-id')`,
		institutionID, firstSyncerID, firstUserID, secondSyncerID, secondUserID); err != nil {
		t.Fatalf("create identity links: %v", err)
	}

	users := NewUserRepository(client)
	candidates, err := users.FindIdentityLinkMatchCandidates(
		ctx, institutionID, firstSyncerID, "external_id", "same-external-id", 2)
	if err != nil {
		t.Fatalf("find scoped identity candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != firstUserID {
		t.Fatalf("candidates = %+v, want only user %d", candidates, firstUserID)
	}
}

func TestUserRoleJobTitleAndDepartmentRelationsUsePrimaryKeys(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "membership-ids")
	aliceID := createUserFixture(t, client, institutionID, "alice")
	bobID := createUserFixture(t, client, institutionID, "bob")

	jobTitles := NewJobTitleRepository(client)
	jobTitle := &repository.JobTitle{InstitutionID: institutionID, Name: "operator", DisplayName: "Operator", IsEnabled: true}
	if _, err := jobTitles.Create(ctx, jobTitle); err != nil {
		t.Fatalf("create job title: %v", err)
	}
	if err := jobTitles.SetMemberIDs(ctx, jobTitle.ID, []int64{aliceID}); err != nil {
		t.Fatalf("set job-title member IDs: %v", err)
	}
	if err := jobTitles.SetJobTitleIDsForUser(ctx, institutionID, bobID, []int64{jobTitle.ID}); err != nil {
		t.Fatalf("set user job-title IDs: %v", err)
	}
	if ids, err := jobTitles.JobTitleIDsForUser(ctx, institutionID, bobID); err != nil {
		t.Fatalf("load user job-title IDs: %v", err)
	} else if len(ids) != 1 || ids[0] != jobTitle.ID {
		t.Fatalf("user job-title IDs = %v", ids)
	}

	roles := NewRoleRepository(client)
	role := &repository.Role{InstitutionID: institutionID, Name: "viewer", DisplayName: "Viewer", IsEnabled: true}
	if _, err := roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := roles.SetDirectRoleIDs(ctx, institutionID, bobID, []int64{role.ID}); err != nil {
		t.Fatalf("set direct role IDs: %v", err)
	}
	if ids, err := roles.DirectRoleIDsForUser(ctx, institutionID, bobID); err != nil {
		t.Fatalf("load direct role IDs: %v", err)
	} else if len(ids) != 1 || ids[0] != role.ID {
		t.Fatalf("direct role IDs = %v", ids)
	}

	departments := NewDepartmentRepository(client)
	department := &repository.Department{InstitutionID: institutionID, Name: "ops", DisplayName: "Ops", IsRoot: true}
	if _, err := departments.Create(ctx, department); err != nil {
		t.Fatalf("create department: %v", err)
	}
	// 部门成员关系直接落库（"管理部门成员"入口已按产品决策移除；这里只验证外键存的是主键）
	if err := client.Exec(ctx, `INSERT INTO user_departments (institution_id, user_id, department_id, is_primary)
		VALUES ($1,$2,$3,TRUE),($1,$4,$3,FALSE)`, institutionID, aliceID, department.ID, bobID); err != nil {
		t.Fatalf("seed department members: %v", err)
	}

	var roleUserID, jobTitleUserID, departmentUsers int64
	if err := client.QueryRow(ctx, `SELECT user_id FROM role_users WHERE role_id=$1`, role.ID).Scan(&roleUserID); err != nil {
		t.Fatalf("load role_users user_id: %v", err)
	}
	if err := client.QueryRow(ctx, `SELECT user_id FROM job_title_members WHERE job_title_id=$1 AND user_id=$2`, jobTitle.ID, bobID).Scan(&jobTitleUserID); err != nil {
		t.Fatalf("load job_title_members user_id: %v", err)
	}
	if err := client.QueryRow(ctx, `SELECT COUNT(*) FROM user_departments WHERE department_id=$1`, department.ID).Scan(&departmentUsers); err != nil {
		t.Fatalf("count department members: %v", err)
	}
	if roleUserID != bobID || jobTitleUserID != bobID || departmentUsers != 2 {
		t.Fatalf("relation IDs not persisted: role=%d job_title=%d department_count=%d", roleUserID, jobTitleUserID, departmentUsers)
	}
}

func TestRoleAndPermissionJobTitleRelationsUseCurrentSchema(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "relation-test")
	aliceID := createUserFixture(t, client, institutionID, "alice")
	bobID := createUserFixture(t, client, institutionID, "bob")

	jobTitles := NewJobTitleRepository(client)
	jobTitle := &repository.JobTitle{InstitutionID: institutionID, Name: "manager", DisplayName: "Manager", IsEnabled: true}
	if _, err := jobTitles.Create(ctx, jobTitle); err != nil {
		t.Fatalf("create job title: %v", err)
	}
	if err := jobTitles.SetMemberIDs(ctx, jobTitle.ID, []int64{aliceID}); err != nil {
		t.Fatalf("set job-title members: %v", err)
	}
	existing, err := jobTitles.ExistingIDs(ctx, institutionID, []int64{jobTitle.ID})
	if err != nil {
		t.Fatalf("check job-title IDs: %v", err)
	}
	if !existing[jobTitle.ID] {
		t.Fatal("existing job title ID was not found")
	}

	roles := NewRoleRepository(client)
	role := &repository.Role{InstitutionID: institutionID, Name: "staff", DisplayName: "Staff", IsEnabled: true}
	if _, err := roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := roles.SetMembers(ctx, role.ID, &repository.RoleRelations{
		UserIDs:     []int64{aliceID},
		JobTitleIDs: []int64{jobTitle.ID},
	}); err != nil {
		t.Fatalf("set role relations: %v", err)
	}
	gotRole, err := roles.GetByName(ctx, institutionID, "staff")
	if err != nil {
		t.Fatalf("load role relations: %v", err)
	}
	if len(gotRole.JobTitles) != 1 || gotRole.JobTitles[0].ID != jobTitle.ID || gotRole.JobTitles[0].Name != "manager" {
		t.Fatalf("role job titles = %v", gotRole.JobTitles)
	}
	parent := &repository.Role{InstitutionID: institutionID, Name: "admin", DisplayName: "Admin", IsEnabled: true}
	if _, err := roles.Create(ctx, parent); err != nil {
		t.Fatalf("create parent role: %v", err)
	}
	if err := roles.SetMembers(ctx, parent.ID, &repository.RoleRelations{
		UserIDs: []int64{bobID},
		RoleIDs: []int64{role.ID},
	}); err != nil {
		t.Fatalf("set parent role relations: %v", err)
	}
	var childRoleID int64
	if err := client.QueryRow(ctx, `SELECT child_role_id FROM role_roles WHERE role_id=$1`, parent.ID).Scan(&childRoleID); err != nil {
		t.Fatalf("load child role primary key: %v", err)
	}
	if childRoleID != role.ID {
		t.Fatalf("child role id = %d, want %d", childRoleID, role.ID)
	}
	affectedUsers, err := roles.AffectedUsersForRoles(ctx, institutionID, []string{"staff"})
	if err != nil {
		t.Fatalf("expand affected role users: %v", err)
	}
	if len(affectedUsers) != 2 || affectedUsers[0] != "alice" || affectedUsers[1] != "bob" {
		t.Fatalf("affected role users = %v", affectedUsers)
	}

	permissions := NewPermissionRepository(client)
	permission := &repository.Permission{
		InstitutionID: institutionID,
		Name:          "read-profile",
		DisplayName:   "Read profile",
		Resources:     []string{"profile"},
		Actions:       []string{"read"},
		Effect:        "Allow",
		IsEnabled:     true,
		State:         "Approved",
	}
	if _, err := permissions.Create(ctx, permission); err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := roles.SetMembers(ctx, role.ID, &repository.RoleRelations{
		UserIDs:       []int64{aliceID},
		JobTitleIDs:   []int64{jobTitle.ID},
		PermissionIDs: []int64{permission.ID},
	}); err != nil {
		t.Fatalf("bind permission to role: %v", err)
	}
	var roleRelationCount int
	if err := client.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM role_job_titles WHERE role_id=$1 AND job_title_id IS NOT NULL) +
		(SELECT COUNT(*) FROM role_permissions WHERE role_id=$1 AND permission_id IS NOT NULL)`,
		role.ID).Scan(&roleRelationCount); err != nil {
		t.Fatalf("check role relation primary keys: %v", err)
	}
	if roleRelationCount != 2 {
		t.Fatalf("role relations did not persist primary keys: %d", roleRelationCount)
	}
	if err := permissions.SetSubjects(ctx, permission.ID, []int64{aliceID}, []int64{jobTitle.ID}, []int64{role.ID}); err != nil {
		t.Fatalf("set permission subjects: %v", err)
	}
	gotPermission, err := permissions.GetByName(ctx, institutionID, "read-profile")
	if err != nil {
		t.Fatalf("load permission subjects: %v", err)
	}
	if len(gotPermission.JobTitles) != 1 || gotPermission.JobTitles[0].ID != jobTitle.ID || gotPermission.JobTitles[0].Name != "manager" {
		t.Fatalf("permission job titles = %v", gotPermission.JobTitles)
	}
	var permissionRelationCount int
	if err := client.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM permission_job_titles WHERE permission_id=$1 AND job_title_id IS NOT NULL) +
		(SELECT COUNT(*) FROM permission_roles WHERE permission_id=$1 AND role_id IS NOT NULL)`,
		permission.ID).Scan(&permissionRelationCount); err != nil {
		t.Fatalf("check permission relation primary keys: %v", err)
	}
	if permissionRelationCount != 2 {
		t.Fatalf("permission relations did not persist primary keys: %d", permissionRelationCount)
	}
	affectedUsers, err = roles.AffectedUsersForPermission(ctx, institutionID, "read-profile")
	if err != nil {
		t.Fatalf("expand permission users: %v", err)
	}
	if len(affectedUsers) != 2 || affectedUsers[0] != "alice" || affectedUsers[1] != "bob" {
		t.Fatalf("affected permission users = %v", affectedUsers)
	}
}

func TestTokenRepositoryUpdatesPrimaryKeys(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "token-test")
	userID := createUserFixture(t, client, institutionID, "alice")

	apps := NewApplicationRepository(client)
	app := &repository.Application{
		InstitutionID: institutionID,
		Code:          "portal",
		Name:          "Portal",
		State:         "active",
		AccessMode:    repository.AccessModeAll,
	}
	if _, err := apps.Create(ctx, app); err != nil {
		t.Fatalf("create application: %v", err)
	}

	tokens := NewTokenRepository(client)
	record := &repository.Token{
		InstitutionID:    institutionID,
		ApplicationID:    &app.ID,
		UserID:           &userID,
		Name:             "refresh-test",
		AccessTokenHash:  tokenhash.Hash("old-access"),
		RefreshTokenHash: tokenhash.Hash("old-refresh"),
		TokenType:        "Bearer",
		GrantType:        "authorization_code",
	}
	if _, err := tokens.Create(ctx, record); err != nil {
		t.Fatalf("create token: %v", err)
	}
	record.Revoked = true
	record.AccessTokenHash = tokenhash.Hash("new-access")
	record.RefreshTokenHash = tokenhash.Hash("new-refresh")
	if err := tokens.Update(ctx, record); err != nil {
		t.Fatalf("update token: %v", err)
	}
	got, err := tokens.GetByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if !got.Revoked || got.AccessTokenHash != tokenhash.Hash("new-access") || got.RefreshTokenHash != tokenhash.Hash("new-refresh") {
		t.Fatalf("token update did not persist: %+v", got)
	}
	items, err := tokens.List(ctx, repository.ListQuery{
		InstitutionID: institutionID,
		Field:         "user_id",
		Value:         fmt.Sprint(userID),
	})
	if err != nil {
		t.Fatalf("filter tokens by user_id: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("tokens by user_id = %d", len(items))
	}
}

func TestRotateRefreshTokenIsAtomic(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "rotation-test")
	userID := createUserFixture(t, client, institutionID, "alice")

	apps := NewApplicationRepository(client)
	app := &repository.Application{
		InstitutionID: institutionID,
		Code:          "portal",
		Name:          "Portal",
		State:         "active",
		AccessMode:    repository.AccessModeAll,
	}
	if _, err := apps.Create(ctx, app); err != nil {
		t.Fatalf("create application: %v", err)
	}

	tokens := NewTokenRepository(client)
	oldRecord := &repository.Token{
		InstitutionID:    institutionID,
		ApplicationID:    &app.ID,
		UserID:           &userID,
		Name:             "old-token",
		AccessTokenHash:  tokenhash.Hash("old-access"),
		RefreshTokenHash: tokenhash.Hash("old-refresh"),
		TokenType:        "Bearer",
		GrantType:        "authorization_code",
	}
	if _, err := tokens.Create(ctx, oldRecord); err != nil {
		t.Fatalf("create old token: %v", err)
	}

	invalidAppID := int64(99999999)
	failed := &repository.Token{
		InstitutionID:    institutionID,
		ApplicationID:    &invalidAppID,
		UserID:           &userID,
		Name:             "failed-rotation",
		AccessTokenHash:  tokenhash.Hash("failed-access"),
		RefreshTokenHash: tokenhash.Hash("failed-refresh"),
		TokenType:        "Bearer",
		GrantType:        "authorization_code",
	}
	if err := tokens.RotateRefreshToken(ctx, oldRecord.ID, failed); err == nil {
		t.Fatal("rotation with invalid application_id should fail")
	}
	oldAfterFailure, err := tokens.GetByID(ctx, oldRecord.ID)
	if err != nil {
		t.Fatalf("reload old token: %v", err)
	}
	if oldAfterFailure.Revoked {
		t.Fatal("failed rotation must not revoke the old token")
	}

	replacement := &repository.Token{
		InstitutionID:    institutionID,
		ApplicationID:    &app.ID,
		UserID:           &userID,
		Name:             "new-token",
		AccessTokenHash:  tokenhash.Hash("new-access"),
		RefreshTokenHash: tokenhash.Hash("new-refresh"),
		TokenType:        "Bearer",
		GrantType:        "authorization_code",
	}
	if err := tokens.RotateRefreshToken(ctx, oldRecord.ID, replacement); err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	oldAfterSuccess, err := tokens.GetByID(ctx, oldRecord.ID)
	if err != nil {
		t.Fatalf("reload rotated old token: %v", err)
	}
	if !oldAfterSuccess.Revoked {
		t.Fatalf("old token was not revoked: %+v", oldAfterSuccess)
	}
	if _, err := tokens.GetByRefreshToken(ctx, app.ID, "new-refresh"); err != nil {
		t.Fatalf("new refresh token not persisted: %v", err)
	}
}

// Regression: the baseline schema must not contain plaintext token columns.
func TestTokensSchemaStoresOnlyHashes(t *testing.T) {
	_, pool := integrationClient(t)
	ctx := context.Background()

	var plaintextColumns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='tokens'
		AND column_name IN ('code','access_token','refresh_token','id_token')`).Scan(&plaintextColumns); err != nil {
		t.Fatalf("inspect tokens columns: %v", err)
	}
	if plaintextColumns != 0 {
		t.Fatalf("tokens schema still contains %d plaintext token columns", plaintextColumns)
	}

	var hashColumns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='tokens'
		AND column_name IN ('code_hash','access_token_hash','refresh_token_hash','id_token_hash')`).Scan(&hashColumns); err != nil {
		t.Fatalf("inspect tokens hash columns: %v", err)
	}
	if hashColumns != 4 {
		t.Fatalf("tokens schema has %d hash columns, want 4", hashColumns)
	}

	var applicationPlaintext int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='applications'
		AND column_name='client_secret'`).Scan(&applicationPlaintext); err != nil {
		t.Fatalf("inspect applications columns: %v", err)
	}
	if applicationPlaintext != 0 {
		t.Fatal("applications schema still contains client_secret plaintext column")
	}
	var applicationHash int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='applications'
		AND column_name='client_secret_hash'`).Scan(&applicationHash); err != nil {
		t.Fatalf("inspect applications hash column: %v", err)
	}
	if applicationHash != 1 {
		t.Fatal("applications schema is missing client_secret_hash")
	}
}

func TestTokenRepositoryHashesAuthorizationCodeAndPersistsNonce(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "code-hash-test")
	userID := createUserFixture(t, client, institutionID, "alice")

	apps := NewApplicationRepository(client)
	app := &repository.Application{
		InstitutionID: institutionID,
		Code:          "portal",
		Name:          "Portal",
		State:         "active",
		AccessMode:    repository.AccessModeAll,
	}
	if _, err := apps.Create(ctx, app); err != nil {
		t.Fatalf("create application: %v", err)
	}

	tokens := NewTokenRepository(client)
	record := &repository.Token{
		InstitutionID: institutionID,
		ApplicationID: &app.ID,
		UserID:        &userID,
		Name:          "code-test",
		CodeHash:      tokenhash.Hash("plain-code"),
		CodeNonce:     "nonce-123",
		CodeExpireIn:  time.Now().Add(time.Minute).Unix(),
		TokenType:     "Bearer",
		GrantType:     "authorization_code",
	}
	if _, err := tokens.Create(ctx, record); err != nil {
		t.Fatalf("create code record: %v", err)
	}

	got, err := tokens.GetByCode(ctx, app.ID, "plain-code")
	if err != nil {
		t.Fatalf("lookup code by plaintext: %v", err)
	}
	if got.CodeNonce != "nonce-123" {
		t.Fatalf("code nonce = %q, want nonce-123", got.CodeNonce)
	}
	consumed, err := tokens.ConsumeCode(ctx, app.ID, "plain-code")
	if err != nil || !consumed {
		t.Fatalf("consume code = %v, %v", consumed, err)
	}
	consumed, err = tokens.ConsumeCode(ctx, app.ID, "plain-code")
	if err != nil {
		t.Fatalf("consume code again: %v", err)
	}
	if consumed {
		t.Fatal("authorization code must be single-use")
	}
}

func TestTokenRepositoryScopesLookupAndRevokesBySession(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "token-scope-test")
	userID := createUserFixture(t, client, institutionID, "alice")

	apps := NewApplicationRepository(client)
	appA := &repository.Application{InstitutionID: institutionID, Code: "app-a", Name: "App A", State: "active", AccessMode: repository.AccessModeAll}
	appB := &repository.Application{InstitutionID: institutionID, Code: "app-b", Name: "App B", State: "active", AccessMode: repository.AccessModeAll}
	if _, err := apps.Create(ctx, appA); err != nil {
		t.Fatalf("create app A: %v", err)
	}
	if _, err := apps.Create(ctx, appB); err != nil {
		t.Fatalf("create app B: %v", err)
	}

	tokens := NewTokenRepository(client)
	recordA := &repository.Token{
		InstitutionID: institutionID, ApplicationID: &appA.ID, UserID: &userID, Name: "token-a",
		CodeHash: tokenhash.Hash("code-a"), RefreshTokenHash: tokenhash.Hash("refresh-a"),
		SessionID: "session-a", TokenType: "Bearer", GrantType: "authorization_code",
	}
	recordB := &repository.Token{
		InstitutionID: institutionID, ApplicationID: &appB.ID, UserID: &userID, Name: "token-b",
		CodeHash: tokenhash.Hash("code-b"), RefreshTokenHash: tokenhash.Hash("refresh-b"),
		SessionID: "session-b", TokenType: "Bearer", GrantType: "authorization_code",
	}
	if _, err := tokens.Create(ctx, recordA); err != nil {
		t.Fatalf("create token A: %v", err)
	}
	if _, err := tokens.Create(ctx, recordB); err != nil {
		t.Fatalf("create token B: %v", err)
	}

	if _, err := tokens.GetByRefreshToken(ctx, appB.ID, "refresh-a"); err == nil {
		t.Fatal("refresh token must not be visible to another application")
	}
	if got, err := tokens.GetByRefreshToken(ctx, appA.ID, "refresh-a"); err != nil || got.ID != recordA.ID {
		t.Fatalf("scoped refresh lookup = %+v, %v", got, err)
	}
	if _, err := tokens.GetByCode(ctx, appB.ID, "code-a"); err == nil {
		t.Fatal("authorization code must not be visible to another application")
	}
	if consumed, err := tokens.ConsumeCode(ctx, appB.ID, "code-a"); err != nil || consumed {
		t.Fatalf("cross-application code consumption = %v, %v", consumed, err)
	}
	if consumed, err := tokens.ConsumeCode(ctx, appA.ID, "code-a"); err != nil || !consumed {
		t.Fatalf("scoped code consumption = %v, %v", consumed, err)
	}

	revoked, err := tokens.RevokeBySession(ctx, "session-a")
	if err != nil || revoked != 1 {
		t.Fatalf("RevokeBySession = %d, %v; want 1", revoked, err)
	}
	got, err := tokens.GetByID(ctx, recordA.ID)
	if err != nil || !got.Revoked {
		t.Fatalf("session token was not revoked: %+v, %v", got, err)
	}
	revoked, err = tokens.RevokeBySession(ctx, "session-a")
	if err != nil || revoked != 0 {
		t.Fatalf("second RevokeBySession = %d, %v; want 0", revoked, err)
	}
}

func TestGetIdentityScopesProviderNameToInstitution(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	orgA := createInstitutionFixture(t, client, "identity-a")
	orgB := createInstitutionFixture(t, client, "identity-b")
	userA := createUserFixture(t, client, orgA, "alice")
	userB := createUserFixture(t, client, orgB, "alice")

	var providerA, providerB int64
	for institutionID, target := range map[int64]*int64{orgA: &providerA, orgB: &providerB} {
		if err := client.QueryRow(ctx, `INSERT INTO providers (institution_id, name, display_name, category, type, state)
			VALUES ($1, 'corp-sso', 'Corp SSO', 'oauth', 'oidc', 'active') RETURNING id`, institutionID).Scan(target); err != nil {
			t.Fatalf("create provider: %v", err)
		}
	}

	if _, err := client.Pool().Exec(ctx, `INSERT INTO user_identities
		(institution_id, user_id, provider_id, provider_user_id)
		VALUES ($1,$2,$3,'same-subject'), ($4,$5,$6,'same-subject')`,
		orgA, userA, providerA, orgB, userB, providerB); err != nil {
		t.Fatalf("create identities: %v", err)
	}

	users := NewUserRepository(client)
	identity, err := users.GetIdentity(ctx, orgB, "corp-sso", "same-subject")
	if err != nil {
		t.Fatalf("load tenant-scoped identity: %v", err)
	}
	if identity.InstitutionID != orgB || identity.UserID != userB {
		t.Fatalf("identity resolved to wrong tenant: %+v", identity)
	}
}

// 回归：SSO 自动建号写入的 attributes 能被同步侧按"同源身份连接"的字段对反查到，
// 且 reversed 查询用 jsonb 包含运算符命中 GIN 索引。
func TestUserIdentityAttributesReverseLookup(t *testing.T) {
	client, _ := integrationClient(t)
	ctx := context.Background()
	institutionID := createInstitutionFixture(t, client, "identity-attributes")
	userID := createUserFixture(t, client, institutionID, "jit-user")

	var providerID int64
	if err := client.QueryRow(ctx, `INSERT INTO providers (institution_id, name, display_name, category, type, state)
		VALUES ($1, 'corp-sso', 'Corp SSO', 'oauth', 'dingtalk', 'active') RETURNING id`,
		institutionID).Scan(&providerID); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if _, err := client.Pool().Exec(ctx, `INSERT INTO user_identities
		(institution_id, user_id, provider_id, provider_user_id, attributes)
		VALUES ($1, $2, $3, 'union-1', '{"unionid":"union-1","mobile":"13800000000"}')`,
		institutionID, userID, providerID); err != nil {
		t.Fatalf("insert identity with attributes: %v", err)
	}

	users := NewUserRepository(client)
	candidates, err := users.FindUsersByIdentityAttribute(ctx, institutionID, providerID, "unionid", "union-1", 2)
	if err != nil {
		t.Fatalf("reverse lookup by attribute: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != userID {
		t.Fatalf("candidates = %+v, want user %d", candidates, userID)
	}
	// 不同值不得命中
	none, err := users.FindUsersByIdentityAttribute(ctx, institutionID, providerID, "unionid", "union-2", 2)
	if err != nil || len(none) != 0 {
		t.Fatalf("unexpected candidates for miss: %+v err=%v", none, err)
	}
}
