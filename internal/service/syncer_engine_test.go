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

// 回归（Spec FR-4）：同一个人挂在多个部门时连接器会返回多条，必须先按源端账号 ID 聚合，
// 否则就会像 2026-10-16 的钉钉同步那样"一个人 6 行"。部门归属取并集。
func TestMergeExternalUsersDedupesByExternalID(t *testing.T) {
	users := []syncer.ExternalUser{
		{ID: "1770611962775942", Username: "1770611962775942", Name: "吴旭平", DepartmentIDs: []string{"101", "102"}},
		{ID: "1770611962775942", Username: "1770611962775942", Name: "吴旭平", DepartmentIDs: []string{"102", "103"}},
		{ID: "17273470128752060", Username: "17273470128752060", Name: "李艳丽", DepartmentIDs: []string{"101"}},
	}
	merged := mergeExternalUsers(users)
	if len(merged) != 2 {
		t.Fatalf("应聚合为 2 个人，实际 %d", len(merged))
	}
	if got := merged[0].DepartmentIDs; len(got) != 3 {
		t.Fatalf("部门归属应取并集（101/102/103），实际 %v", got)
	}
	// 幂等：再聚合一次结果不变
	again := mergeExternalUsers(merged)
	if len(again) != 2 || len(again[0].DepartmentIDs) != 3 {
		t.Fatalf("重复聚合应稳定，实际 %d 条 / %v", len(again), again[0].DepartmentIDs)
	}
}

// 回归（Spec Q4.5）：源端根部门占位（钉钉/企微 1、飞书 0）不落库为部门，
// 其子节点成为部门树的顶层（挂到同步目标 / 组织根），而不是挂在名为 "1" 的节点下。
func TestPlanDepartmentsSkipsRootPlaceholder(t *testing.T) {
	svc := &SyncerService{}
	root := &repository.Department{ID: 9, InstitutionID: 1, Name: "built-in", DisplayName: "Polaris", IsRoot: true, CodePath: "built-in"}
	plan := &syncPlan{
		deptByName:  map[string]*repository.Department{root.Name: root},
		deptByID:    map[int64]*repository.Department{root.ID: root},
		deptLinks:   map[string]int64{},
		deptNames:   map[string]string{},
		deptRootIDs: map[string]bool{},
		rootID:      root.ID,
	}
	groups := []syncer.ExternalGroup{
		{ID: "1", Name: "1", IsRoot: true},
		{ID: "120", Name: "管理层", ParentID: "1"},
		{ID: "121", Name: "技术产品部", ParentID: "1"},
		{ID: "131", Name: "产品研发", ParentID: "121"},
	}
	svc.planDepartments(plan, groups)

	if len(plan.deptCreate) != 3 {
		t.Fatalf("根占位不落库，应新建 3 个部门，实际 %d: %+v", len(plan.deptCreate), plan.deptCreate)
	}
	for _, g := range plan.deptCreate {
		if g.ID == "1" {
			t.Fatal("源端根占位节点不得进入新建集合")
		}
	}
	// 依赖顺序：父先于子（产品研发依赖技术产品部）
	if plan.deptCreate[len(plan.deptCreate)-1].ID != "131" {
		t.Fatalf("父节点必须先于子节点创建，实际顺序: %+v", plan.deptCreate)
	}
	// 顶层节点的父部门 = 组织根
	parent := svc.desiredDeptParent(plan, map[string]syncer.ExternalGroup{
		"1": {ID: "1", IsRoot: true}, "120": {ID: "120", Name: "管理层", ParentID: "1"},
	}, syncer.ExternalGroup{ID: "120", Name: "管理层", ParentID: "1"})
	if parent == nil || parent.ID != root.ID {
		t.Fatalf("根占位节点的子节点应挂到组织根，实际 %+v", parent)
	}
}

// 回归（Spec FR-1）：本地标识名冲突时（同组织内不同父级下存在同名部门）以源端 ID 消歧，
// 保证"本地名唯一（迁移 039 的唯一约束）"且结果确定、幂等。
func TestPlanDepartmentsDisambiguatesDuplicateNames(t *testing.T) {
	svc := &SyncerService{}
	existing := &repository.Department{ID: 5, InstitutionID: 1, Name: "市场部", DisplayName: "市场部"}
	plan := &syncPlan{
		deptByName:  map[string]*repository.Department{existing.Name: existing},
		deptByID:    map[int64]*repository.Department{existing.ID: existing},
		deptLinks:   map[string]int64{},
		deptNames:   map[string]string{},
		deptRootIDs: map[string]bool{},
	}
	groups := []syncer.ExternalGroup{
		{ID: "138", Name: "市场部", ParentID: "123"},
		{ID: "143", Name: "市场部", ParentID: "125"},
	}
	svc.planDepartments(plan, groups)

	names := map[string]bool{}
	for _, g := range plan.deptCreate {
		names[plan.deptNames[g.ID]] = true
	}
	if len(names) != 2 {
		t.Fatalf("两个同名外部部门应消歧为不同的本地标识名，实际 %v", plan.deptNames)
	}
	for name := range names {
		if name == "市场部" {
			t.Fatalf("与既有部门重名时不得直接沿用原名，实际 %v", plan.deptNames)
		}
	}
}

// 回归：多值部门归属集合比较（顺序无关；nil 与空集合等价）
func TestSameIDSet(t *testing.T) {
	if !sameIDSet([]int64{1, 2}, []int64{2, 1}) {
		t.Fatal("集合相同（顺序不同）应判定相等")
	}
	if !sameIDSet(nil, []int64{}) {
		t.Fatal("nil 与空集合等价")
	}
	if sameIDSet([]int64{1}, []int64{1, 2}) {
		t.Fatal("元素个数不同应判定不等")
	}
}
