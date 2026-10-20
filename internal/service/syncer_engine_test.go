package service

import (
	"context"
	"fmt"
	"testing"

	"polaris/internal/repository"
	"polaris/internal/service/syncer"
)

// 回归（2026-10 多部门问题）：只勾「同步用户」时也必须规划部门树，
// 否则 dept_id_list 没有本地映射，用户的第二、第三个部门会被静默丢弃。
func TestNeedsDepartmentPlanForUserSync(t *testing.T) {
	cases := []struct {
		name string
		sy   *repository.Syncer
		want bool
	}{
		{"都不开", &repository.Syncer{}, false},
		{"只同步部门", &repository.Syncer{SyncGroups: true}, true},
		{"只同步用户", &repository.Syncer{SyncUsers: true}, true},
		{"都开启", &repository.Syncer{SyncUsers: true, SyncGroups: true}, true},
	}
	for _, tc := range cases {
		if got := needsDepartmentPlan(tc.sy); got != tc.want {
			t.Fatalf("%s: needsDepartmentPlan=%v，期望 %v", tc.name, got, tc.want)
		}
	}
}

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

func TestDepartmentEntityNameMapping(t *testing.T) {
	plan := &syncPlan{
		departmentMapping: map[string]string{"name": "name"},
		deptByName:        map[string]*repository.Department{},
		deptLinks:         map[string]int64{},
		deptNames:         map[string]string{},
	}
	group := syncer.ExternalGroup{ID: "101", Name: "fallback", Raw: map[string]string{"name": "技术部"}}
	if got := plan.localDeptName(group); got != "技术部" {
		t.Fatalf("部门名称应取实体映射字段，实际 %q", got)
	}
	if got := plan.deptDisplayName(group, "技术部"); got != "技术部" {
		t.Fatalf("部门显示名应取实体映射字段，实际 %q", got)
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

// fakeIdentityUserRepo 只实现反向认领所需的属性查询，其余经嵌入接口兜底（不会被调用）。
type fakeIdentityUserRepo struct {
	repository.UserRepository
	candidates map[string][]*repository.User
	lastAttr   string
}

func (f *fakeIdentityUserRepo) FindUsersByIdentityAttribute(
	_ context.Context, _ int64, providerID int64, attribute, value string, _ int,
) ([]*repository.User, error) {
	f.lastAttr = attribute
	return f.candidates[fmt.Sprintf("%d|%s|%s", providerID, attribute, value)], nil
}

type fakeBoundProviderRepo struct {
	repository.ProviderRepository
	items []*repository.Provider
}

func (f *fakeBoundProviderRepo) ListIdentityLinkProviders(context.Context, int64, int64) ([]*repository.Provider, error) {
	return f.items, nil
}

// 回归：SSO 先建号、通讯录随后同步——同步侧必须按"同源身份连接"的字段对认领同一用户，
// 而不是再建一条（这正是钉钉等源重复建号的根因）。
func TestPlanUsersAdoptsSSOCreatedUserByIdentityLink(t *testing.T) {
	existing := &repository.User{ID: 77, InstitutionID: 3, Name: "jit-user"}
	users := &fakeIdentityUserRepo{candidates: map[string][]*repository.User{
		"5|unionid|ding-union-1": {existing},
	}}
	providers := &fakeBoundProviderRepo{items: []*repository.Provider{
		{ID: 5, MatchType: "identity_link", MatchSyncerID: 9,
			MatchProviderField: "unionid", MatchSyncerField: "unionid"},
	}}
	svc := &SyncerService{users: users, providers: providers}

	plan := newTestSyncPlan()
	sy := &repository.Syncer{ID: 9, InstitutionID: 3, Name: "dingtalk-conn"}
	err := svc.planUsers(context.Background(), sy, plan, []syncer.ExternalUser{
		{ID: "ext-1", Username: "完全不同的名字", Raw: map[string]string{"unionid": "ding-union-1"}},
	})
	if err != nil {
		t.Fatalf("planUsers: %v", err)
	}
	if len(plan.userCreate) != 0 {
		t.Fatalf("应认领已有用户而不是新建: %+v", plan.userCreate)
	}
	if plan.userLinks["ext-1"] != 77 {
		t.Fatalf("userLinks = %v, want ext-1 -> 77", plan.userLinks)
	}
	if len(plan.userTouched) != 1 || plan.userTouched[0].local.ID != 77 {
		t.Fatalf("命中用户未进入落地列表: %+v", plan.userTouched)
	}
	if users.lastAttr != "unionid" {
		t.Fatalf("反查属性 = %q, want unionid", users.lastAttr)
	}
}

func newTestSyncPlan() *syncPlan {
	return &syncPlan{
		deptByName:    map[string]*repository.Department{},
		deptByID:      map[int64]*repository.Department{},
		deptLinks:     map[string]int64{},
		deptNames:     map[string]string{},
		deptRootIDs:   map[string]bool{},
		userByName:    map[string]*repository.User{},
		userByID:      map[int64]*repository.User{},
		userLinks:     map[string]int64{},
		userDepts:     map[int64][]int64{},
		jobTitleByKey: map[string]*repository.JobTitle{},
		jobTitleByID:  map[int64]*repository.JobTitle{},
		jobTitleLinks: map[string]int64{},
		jobTitleRaw:   map[string]string{},
		userJobTitles: map[string][]string{},
	}
}
