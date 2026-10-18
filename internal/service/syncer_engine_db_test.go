package service

import (
	"context"
	"os"
	"testing"

	"polaris/internal/config"
	"polaris/internal/repository"
	"polaris/internal/repository/postgres"
	"polaris/internal/service/syncer"
	"polaris/pkg/dbclient"
)

// 本文件是**需要真实数据库**的集成回归（Spec 第六阶段 FR-1 / FR-4 / Q2 / Q4.5）。
//
// 为什么值得留：2026-10-16 的钉钉同步事故（一个人 6 行、部门树顶层变成 "1"）
// 正是"本地对象锚定"与"多部门归属"这两条链路的组合缺陷——单元测试锁不住，
// 必须在真库上跑一遍"同步 → 再同步"才能证明幂等。
//
// 默认跳过；本地验证：
//
//	POLARIS_TEST_DB=1 go test ./internal/service -run TestSyncEngineIdempotent_RequiresDB -v
func TestSyncEngineIdempotent_RequiresDB(t *testing.T) {
	if os.Getenv("POLARIS_TEST_DB") != "1" {
		t.Skip("需要真实数据库：设置 POLARIS_TEST_DB=1 并确保 configs/app.yaml 指向可写库")
	}
	ctx := context.Background()
	manager := config.NewManager()
	if err := manager.LoadFromFile("../../configs/app.yaml"); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	cfg := manager.Get()
	client, err := dbclient.New(dbclient.Config{DSN: cfg.Database.DSN(), MaxOpenConns: 4, MaxIdleConns: 1})
	if err != nil {
		t.Fatalf("连接数据库失败: %v", err)
	}
	defer client.Close()

	orgRepo := postgres.NewInstitutionRepository(client)
	userRepo := postgres.NewUserRepository(client)
	deptRepo := postgres.NewDepartmentRepository(client)
	syncerRepo := postgres.NewSyncerRepository(client)
	linkRepo := postgres.NewSyncerLinkRepository(client)
	userDeptRepo := postgres.NewUserDepartmentRepository(client)

	org, err := orgRepo.GetByName(ctx, "built-in")
	if err != nil {
		t.Fatalf("内置组织不存在（先启动一次服务完成 seed）: %v", err)
	}

	sy := &repository.Syncer{InstitutionID: org.ID, Name: "db-test-syncer", Type: "dingtalk", SyncUsers: true, SyncGroups: true}
	id, err := syncerRepo.Create(ctx, sy)
	if err != nil {
		t.Fatalf("创建同步器失败: %v", err)
	}
	sy.ID = id
	defer func() {
		_ = client.Exec(ctx, `DELETE FROM syncer_user_links WHERE syncer_id=$1`, sy.ID)
		_ = client.Exec(ctx, `DELETE FROM syncer_department_links WHERE syncer_id=$1`, sy.ID)
		_ = client.Exec(ctx, `DELETE FROM user_departments WHERE syncer_id=$1`, sy.ID)
		_ = client.Exec(ctx, `DELETE FROM users WHERE register_source=$1`, sy.Name)
		_ = client.Exec(ctx, `DELETE FROM job_title_members WHERE job_title_id IN (SELECT id FROM job_titles WHERE name LIKE 'db-test-%')`)
		_ = client.Exec(ctx, `DELETE FROM syncer_job_title_links WHERE syncer_id=$1`, sy.ID)
		_ = client.Exec(ctx, `DELETE FROM job_titles WHERE name LIKE 'db-test-%'`)
		_ = client.Exec(ctx, `DELETE FROM departments WHERE name LIKE 'db-test-%'`)
		_ = client.Exec(ctx, `DELETE FROM syncers WHERE id=$1`, sy.ID)
	}()

	connector := &fakeDirectoryConnector{}
	svc := NewSyncerService(syncerRepo, userRepo, deptRepo, linkRepo, userDeptRepo, postgres.NewJobTitleRepository(client), orgRepo, nil, &AuditRecorder{})

	// ① 首次：1 个源端用户在两个部门各出现一次（钉钉就是这么返回的）
	plan, err := svc.buildPlan(ctx, sy, connector, sy)
	if err != nil {
		t.Fatalf("构建计划失败: %v", err)
	}
	if len(plan.deptCreate) != 2 {
		t.Fatalf("应为 2 个子部门建行、根占位不落库，实际 %d: %+v", len(plan.deptCreate), plan.deptCreate)
	}
	if len(plan.userCreate) != 1 {
		t.Fatalf("同一人出现在两个部门也应只建 1 个账号，实际 %d", len(plan.userCreate))
	}
	counts, errs := svc.applyPlan(ctx, sy, plan, sy)
	if len(errs) > 0 {
		t.Fatalf("落地失败: %v", errs)
	}
	if counts.Created != 4 { // 2 部门 + 1 账号 + 1 派生职务
		t.Fatalf("首次同步应创建 4 个对象（2 部门 + 1 账号 + 1 职务），实际 %+v", counts)
	}

	// ② 断言落库结果：1 个账号、2 条部门归属、映射行齐备、顶层部门挂在组织根下
	local, err := userRepo.GetByName(ctx, org.ID, "db-test-001")
	if err != nil {
		t.Fatalf("账号未创建: %v", err)
	}
	deptIDs, err := userDeptRepo.DepartmentIDsForUser(ctx, local.ID)
	if err != nil {
		t.Fatalf("读取部门归属失败: %v", err)
	}
	if len(deptIDs) != 2 {
		t.Fatalf("应写入 2 条部门归属（多值），实际 %v", deptIDs)
	}
	if local.DepartmentID == nil || *local.DepartmentID != deptIDs[0] {
		t.Fatalf("users.department_id 应等于主部门 %v，实际 %v", deptIDs[0], local.DepartmentID)
	}
	userLinks, err := linkRepo.LoadExternalIndex(ctx, "user", sy.ID)
	if err != nil || userLinks["db-test-001"] != local.ID {
		t.Fatalf("账号映射行缺失: %v / %v", userLinks, err)
	}
	deptLinks, err := linkRepo.LoadExternalIndex(ctx, "department", sy.ID)
	if err != nil || len(deptLinks) != 2 {
		t.Fatalf("部门映射行应为 2 条，实际 %v / %v", deptLinks, err)
	}
	root, err := deptRepo.GetRoot(ctx, org.ID)
	if err != nil {
		t.Fatalf("组织根部门缺失: %v", err)
	}
	for _, deptID := range deptIDs {
		dept, err := deptRepo.GetByID(ctx, deptID)
		if err != nil {
			t.Fatalf("部门不存在: %v", err)
		}
		if dept.ParentID == nil || *dept.ParentID != root.ID {
			t.Fatalf("顶层部门应挂在组织根之下（源端占位根 1 不落库），实际 parent=%v", dept.ParentID)
		}
	}

	// ③ 再同步一次：必须完全幂等（不新建、不新增归属行）
	plan2, err := svc.buildPlan(ctx, sy, connector, sy)
	if err != nil {
		t.Fatalf("第二次构建计划失败: %v", err)
	}
	if len(plan2.deptCreate) != 0 || len(plan2.userCreate) != 0 {
		t.Fatalf("第二次不应再新建任何对象：部门 %d、账号 %d", len(plan2.deptCreate), len(plan2.userCreate))
	}
	counts2, errs2 := svc.applyPlan(ctx, sy, plan2, sy)
	if len(errs2) > 0 {
		t.Fatalf("第二次落地失败: %v", errs2)
	}
	if counts2.Created != 0 {
		t.Fatalf("第二次同步不应有创建，实际 %+v", counts2)
	}
	// 职务：源端只给文本（Title），本地必须派生实体 + 任职关系（而不是只写 users.title）
	var titleName string
	if err := client.QueryRow(ctx, `SELECT t.name FROM job_titles t
		JOIN job_title_members m ON m.job_title_id = t.id
		JOIN users u ON u.id = m.user_id
		WHERE u.name = 'db-test-001'`).Scan(&titleName); err != nil {
		t.Fatalf("同步后应派生职务并建立任职关系: %v", err)
	}
	jobTitleLinks, err := linkRepo.LoadExternalIndex(ctx, "job_title", sy.ID)
	if err != nil || len(jobTitleLinks) == 0 {
		t.Fatalf("职务映射行缺失: %v / %v", jobTitleLinks, err)
	}

	var userCount int
	if err := client.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE institution_id=$1 AND name LIKE 'db-test-%'`, org.ID).Scan(&userCount); err != nil {
		t.Fatalf("统计账号失败: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("重复同步后本地账号应恒为 1 行，实际 %d", userCount)
	}
}

// fakeDirectoryConnector 模拟"按部门逐条返回、且带源端根占位节点"的通讯录连接器（钉钉/企微/飞书的共同形态）。
type fakeDirectoryConnector struct{}

func (c *fakeDirectoryConnector) Type() string { return "dingtalk" }

func (c *fakeDirectoryConnector) FetchGroups(_ context.Context, _ *repository.Syncer) ([]syncer.ExternalGroup, error) {
	return []syncer.ExternalGroup{
		{ID: "1", Name: "1", IsRoot: true},
		{ID: "db-test-d1", Name: "db-test-研发部", ParentID: "1"},
		{ID: "db-test-d2", Name: "db-test-产品部", ParentID: "1"},
	}, nil
}

func (c *fakeDirectoryConnector) FetchUsers(_ context.Context, _ *repository.Syncer) ([]syncer.ExternalUser, error) {
	// 同一个人在两个部门各出现一次 —— 事故现场就是这么来的
	return []syncer.ExternalUser{
		{ID: "db-test-001", Username: "db-test-001", Name: "重复用户", Title: "db-test-项目经理",
			DepartmentIDs: []string{"db-test-d1"},
			Raw:           map[string]string{"userid": "db-test-001", "name": "重复用户", "title": "db-test-项目经理"}},
		{ID: "db-test-001", Username: "db-test-001", Name: "重复用户", Title: "db-test-项目经理",
			DepartmentIDs: []string{"db-test-d2"},
			Raw:           map[string]string{"userid": "db-test-001", "name": "重复用户", "title": "db-test-项目经理"}},
	}, nil
}
