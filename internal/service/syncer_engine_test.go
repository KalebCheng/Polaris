package service

import (
	"testing"

	"polaris/internal/repository"
	"polaris/internal/service/syncer"
)

// 回归：同步保护阈值——达到阈值即取消任务；0 表示不限制（Spec FR-14）。
func TestGuardTripped(t *testing.T) {
	plan := &syncPlan{userDisable: []*repository.User{{}, {}, {}}}
	if tripped, _ := guardTripped(plan, repository.SyncGuard{MaxUserDeletes: 0}); tripped {
		t.Fatal("阈值 0 表示不限制，不应取消任务")
	}
	if tripped, _ := guardTripped(plan, repository.SyncGuard{MaxUserDeletes: 4}); tripped {
		t.Fatal("3 < 4，不应取消任务")
	}
	tripped, message := guardTripped(plan, repository.SyncGuard{MaxUserDeletes: 3})
	if !tripped || message == "" {
		t.Fatalf("达到阈值应取消任务并给出原因，实际 tripped=%v message=%q", tripped, message)
	}
}

// 回归：外部组织按父节点还原层级，父节点先建；挂不上父节点的（父不在本次集合）落到同步目标之下。
func TestPlanDepartmentsBuildsTree(t *testing.T) {
	svc := &SyncerService{}
	existing := &repository.Department{ID: 7, Name: "研发中心", DisplayName: "研发中心", CodePath: "研发中心"}
	plan := &syncPlan{
		deptByName:    map[string]*repository.Department{existing.Name: existing},
		deptIDByName:  map[string]int64{existing.Name: existing.ID},
		deptLocalName: map[string]string{},
	}
	groups := []syncer.ExternalGroup{
		{ID: "d2", Name: "后端组", ParentID: "d1"},
		{ID: "d1", Name: "研发中心"},                // 已存在
		{ID: "d3", Name: "外包组", ParentID: "d9"}, // 父不在集合内 → 扁平
	}
	svc.planDepartments(plan, &repository.Syncer{}, groups)

	if len(plan.deptCreate) != 2 {
		t.Fatalf("应新建 2 个部门，实际 %d", len(plan.deptCreate))
	}
	// 依赖顺序：父先于子（后端组依赖 d1，d1 已存在；外包组无父）
	names := []string{
		sanitizeName(plan.deptCreate[0].Name),
		sanitizeName(plan.deptCreate[1].Name),
	}
	has := func(target string) bool { return names[0] == target || names[1] == target }
	if !has("后端组") || !has("外包组") {
		t.Fatalf("新建集合错误: %v", names)
	}
	if plan.deptLocalName["d2"] != "后端组" || plan.deptLocalName["d3"] != "外包组" {
		t.Fatalf("外部标识到本地组织名的映射缺失: %v", plan.deptLocalName)
	}
	// 已存在的部门：父节点未变，不应进入更新集合
	if len(plan.deptUpdate) != 0 {
		t.Fatalf("父节点未变化时不应更新: %v", plan.deptUpdate)
	}
}

// 回归：外部层级变化（父节点不同）时进入更新集合，且深度计算用于稳定排序。
func TestPlanDepartmentsDetectsMove(t *testing.T) {
	svc := &SyncerService{}
	parent := &repository.Department{ID: 3, Name: "总部", DisplayName: "总部", CodePath: "总部"}
	child := &repository.Department{ID: 4, Name: "华东", DisplayName: "华东", ParentID: nil, CodePath: "华东"}
	plan := &syncPlan{
		deptByName:    map[string]*repository.Department{parent.Name: parent, child.Name: child},
		deptIDByName:  map[string]int64{parent.Name: parent.ID, child.Name: child.ID},
		deptLocalName: map[string]string{},
	}
	groups := []syncer.ExternalGroup{
		{ID: "d1", Name: "总部"},
		{ID: "d2", Name: "华东", ParentID: "d1"},
	}
	svc.planDepartments(plan, &repository.Syncer{}, groups)
	if len(plan.deptUpdate) != 1 {
		t.Fatalf("父节点变化应进入更新集合，实际 %v", plan.deptUpdate)
	}
}

// 回归：试运行预览的计数与样例来自同一份计划（试运行/正式运行不漂移）。
func TestPreviewCounts(t *testing.T) {
	svc := &SyncerService{}
	plan := &syncPlan{
		userCreate:  []syncer.ExternalUser{{Username: "alice"}, {Username: "bob"}},
		userUpdate:  []syncPlanPortrait{{local: &repository.User{Name: "carol"}}},
		userSkip:    5,
		userDisable: []*repository.User{{Name: "dave"}, {Name: "erin"}, {Name: "frank"}},
		deptCreate:  []syncer.ExternalGroup{{ID: "d1", Name: "研发"}},
		warnings:    []string{"fetch departments failed: boom"},
	}
	preview := svc.preview(plan, repository.SyncGuard{MaxUserDeletes: 10})
	if preview.UserCreate != 2 || preview.UserUpdate != 1 || preview.UserSkip != 5 || preview.UserDisable != 3 {
		t.Fatalf("账号差异计数错误: %+v", preview)
	}
	if preview.DeptCreate != 1 {
		t.Fatalf("组织差异计数错误: %+v", preview)
	}
	if preview.GuardTripped {
		t.Fatal("3 < 10，不应触发同步保护")
	}
	if len(preview.Samples) == 0 || len(preview.Warnings) != 1 {
		t.Fatalf("样例与告警应随预览返回: %+v", preview)
	}
}

// 回归：只比较外部提供了值的字段——外部为空不得把本地数据清空。
func TestUserNeedsUpdate(t *testing.T) {
	local := &repository.User{
		Name: "alice", DisplayName: "Alice", Email: "alice@example.com", Phone: "13800000000",
	}
	if userNeedsUpdate(local, syncer.ExternalUser{Username: "alice"}, &syncPlan{}) {
		t.Fatal("外部字段为空时不应判定为需要更新（否则会清空本地数据）")
	}
	if !userNeedsUpdate(local, syncer.ExternalUser{Username: "alice", Email: "new@example.com"}, &syncPlan{}) {
		t.Fatal("邮箱变化应判定为需要更新")
	}
	// 组织归属变化
	deptID := int64(9)
	plan := &syncPlan{
		deptLocalName: map[string]string{"d1": "研发中心"},
		deptIDByName:  map[string]int64{"研发中心": deptID},
	}
	if !userNeedsUpdate(local, syncer.ExternalUser{Username: "alice", DepartmentIDs: []string{"d1"}}, plan) {
		t.Fatal("组织归属变化应判定为需要更新")
	}
	local.DepartmentID = &deptID
	if userNeedsUpdate(local, syncer.ExternalUser{Username: "alice", DepartmentIDs: []string{"d1"}}, plan) {
		t.Fatal("组织归属未变化时不应判定为需要更新")
	}
}

// 回归：外部组织 ID 未解析出本地部门时不写归属（避免写入悬空 ID）。
func TestLocalDepartmentID(t *testing.T) {
	plan := &syncPlan{
		deptLocalName: map[string]string{"d1": "研发中心"},
		deptIDByName:  map[string]int64{"研发中心": 12},
	}
	if got := plan.localDepartmentID(syncer.ExternalUser{DepartmentIDs: []string{"unknown"}}); got != 0 {
		t.Fatalf("未解析的组织应返回 0，实际 %d", got)
	}
	if got := plan.localDepartmentID(syncer.ExternalUser{DepartmentIDs: []string{"unknown", "d1"}}); got != 12 {
		t.Fatalf("应回退到已解析的组织，实际 %d", got)
	}
}
