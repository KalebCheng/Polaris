package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/syncer"
)

// 运行触发来源（写入 syncer_runs.trigger）
const (
	SyncTriggerManual   = "manual"
	SyncTriggerSchedule = "schedule"
	SyncTriggerRetry    = "retry"
)

// 运行状态（写入 syncer_runs.status / syncers.last_status）
const (
	SyncStatusSuccess = "success"
	SyncStatusFailed  = "failed"
	SyncStatusSkipped = "skipped" // 窗口内已执行 / 资源占用中 → 本次跳过
	SyncStatusAborted = "aborted" // 同步保护触发 → 取消本次任务
)

// maxRunErrors 单次运行最多记录的失败明细条数
const maxRunErrors = 200

// SyncPreview 试运行结果（不写任何数据）
type SyncPreview struct {
	UserCreate   int      `json:"user_create"`
	UserUpdate   int      `json:"user_update"`
	UserSkip     int      `json:"user_skip"`
	UserDisable  int      `json:"user_disable"`
	DeptCreate   int      `json:"dept_create"`
	DeptUpdate   int      `json:"dept_update"`
	Samples      []string `json:"samples,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	GuardTripped bool     `json:"guard_tripped"`
	GuardMessage string   `json:"guard_message,omitempty"`
}

// syncPlanPortrait 需要更新的本地账号及其外部对应项
type syncPlanPortrait struct {
	local *repository.User
	ext   syncer.ExternalUser
}

// syncPlan 一次同步的差异计划。
// **试运行与正式运行共用同一份计算**（否则两套逻辑必然漂移——Spec FR-14）。
type syncPlan struct {
	depts         []syncer.ExternalGroup
	deptCreate    []syncer.ExternalGroup
	deptUpdate    []syncer.ExternalGroup
	deptByName    map[string]*repository.Department
	deptIDByName  map[string]int64
	deptLocalName map[string]string // 外部组织 ID → 本地组织名

	users       []syncer.ExternalUser
	userCreate  []syncer.ExternalUser
	userUpdate  []syncPlanPortrait
	userSkip    int
	userDisable []*repository.User

	warnings []string
}

// buildPlan 拉取外部数据并计算差异（不写库）。
func (s *SyncerService) buildPlan(ctx context.Context, sy *repository.Syncer, connector syncer.Connector, work *repository.Syncer) (*syncPlan, error) {
	plan := &syncPlan{
		deptByName:    map[string]*repository.Department{},
		deptIDByName:  map[string]int64{},
		deptLocalName: map[string]string{},
	}

	existing, err := s.departments.List(ctx, repository.ListQuery{InstitutionID: sy.InstitutionID, Limit: 2000})
	if err != nil {
		return nil, err
	}
	for _, d := range existing {
		plan.deptByName[d.Name] = d
		plan.deptIDByName[d.Name] = d.ID
	}

	if sy.SyncGroups {
		groups, err := connector.FetchGroups(ctx, work)
		if err != nil {
			// 组织拉取失败不阻断账号同步，但要如实记录告警（试运行与运行记录都会带上）
			plan.warnings = append(plan.warnings, "fetch departments failed: "+err.Error())
			logger.Warn("syncer fetch departments failed", "syncer", sy.Name, "error", err)
		} else {
			plan.depts = groups
			s.planDepartments(plan, sy, groups)
		}
	}

	if sy.SyncUsers {
		users, err := connector.FetchUsers(ctx, work)
		if err != nil {
			return nil, err
		}
		plan.users = users
		if err := s.planUsers(ctx, sy, plan, users); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

// planDepartments 计算组织（部门）树的增改：按外部父节点还原层级，缺失父节点则挂到「同步目标」之下。
func (s *SyncerService) planDepartments(plan *syncPlan, sy *repository.Syncer, groups []syncer.ExternalGroup) {
	byExternalID := make(map[string]syncer.ExternalGroup, len(groups))
	for _, g := range groups {
		byExternalID[g.ID] = g
	}
	visited := map[string]bool{}
	var visit func(g syncer.ExternalGroup, depth int) string
	visit = func(g syncer.ExternalGroup, depth int) string {
		name := sanitizeName(firstNonEmptyStr(g.Name, g.ID))
		if name == "" {
			return ""
		}
		plan.deptLocalName[g.ID] = name
		if visited[g.ID] || depth > 12 { // 断环与深度保护
			return name
		}
		visited[g.ID] = true
		local, exists := plan.deptByName[name]
		if !exists {
			plan.deptCreate = append(plan.deptCreate, g)
			return name
		}
		// 已存在：父节点不同则需要移动（按外部层级还原）
		if parent := s.resolveLocalParent(plan, byExternalID, g, depth); parent != nil && !deptParentMatches(local, parent) {
			plan.deptUpdate = append(plan.deptUpdate, g)
		}
		return name
	}
	for _, g := range groups {
		visit(g, 0)
	}
	// 依赖顺序：父先于子（父不在本次集合内的先建）
	sort.SliceStable(plan.deptCreate, func(i, j int) bool {
		return s.deptDepth(byExternalID, plan.deptCreate[i]) < s.deptDepth(byExternalID, plan.deptCreate[j])
	})
}

// resolveLocalParent 解析外部父节点对应的本地部门（nil 表示挂在同步目标 / 组织根之下）
func (s *SyncerService) resolveLocalParent(
	plan *syncPlan, byExternalID map[string]syncer.ExternalGroup, g syncer.ExternalGroup, depth int,
) *repository.Department {
	if g.ParentID == "" {
		return nil
	}
	parent, ok := byExternalID[g.ParentID]
	if !ok {
		return nil
	}
	name := sanitizeName(firstNonEmptyStr(parent.Name, parent.ID))
	if name == "" {
		return nil
	}
	return plan.deptByName[name]
}

func (s *SyncerService) deptDepth(byExternalID map[string]syncer.ExternalGroup, g syncer.ExternalGroup) int {
	depth := 0
	seen := map[string]bool{}
	for cur := g; depth < 12; depth++ {
		if cur.ParentID == "" || seen[cur.ID] {
			break
		}
		parent, ok := byExternalID[cur.ParentID]
		if !ok {
			break
		}
		seen[cur.ID] = true
		cur = parent
	}
	return depth
}

// deptParentMatches 判断本地部门的父节点是否与外部层级一致（与 department_service 的 sameParent 区分）
func deptParentMatches(local *repository.Department, parent *repository.Department) bool {
	if parent == nil {
		return local.ParentID == nil
	}
	return local.ParentID != nil && *local.ParentID == parent.ID
}

// planUsers 计算账号的增/改/跳过，并在开启"源端缺失即停用"时计算需要停用的本地账号。
func (s *SyncerService) planUsers(ctx context.Context, sy *repository.Syncer, plan *syncPlan, users []syncer.ExternalUser) error {
	seen := make(map[string]bool, len(users))
	for _, ext := range users {
		name := sanitizeName(ext.Username)
		if name == "" {
			plan.userSkip++
			continue
		}
		seen[name] = true
		local, err := s.users.GetByName(ctx, sy.InstitutionID, name)
		if err != nil {
			plan.userCreate = append(plan.userCreate, ext)
			continue
		}
		if userNeedsUpdate(local, ext, plan) {
			plan.userUpdate = append(plan.userUpdate, syncPlanPortrait{local: local, ext: ext})
		} else {
			plan.userSkip++
		}
	}

	if !sy.SoftDelete {
		return nil
	}
	// 只处理"由本数据源同步而来"的账号，绝不触碰管理员手工创建的账号
	localUsers, err := s.users.List(ctx, repository.ListQuery{
		InstitutionID: sy.InstitutionID, Field: "register_source", Value: sy.Name, Limit: 5000,
	})
	if err != nil {
		return err
	}
	for _, local := range localUsers {
		if seen[local.Name] || local.IsForbidden || local.IsDeleted {
			continue
		}
		plan.userDisable = append(plan.userDisable, local)
	}
	return nil
}

// userNeedsUpdate 判断本地账号是否需要更新（只比较外部提供了值的字段，避免清空本地数据）
func userNeedsUpdate(local *repository.User, ext syncer.ExternalUser, plan *syncPlan) bool {
	if ext.Name != "" && local.DisplayName != ext.Name {
		return true
	}
	if ext.Email != "" && local.Email != ext.Email {
		return true
	}
	if ext.Phone != "" && local.Phone != ext.Phone {
		return true
	}
	if ext.Avatar != "" && local.Avatar != ext.Avatar {
		return true
	}
	if ext.Title != "" && local.Title != ext.Title {
		return true
	}
	if ext.Location != "" && local.Location != ext.Location {
		return true
	}
	if ext.CountryCode != "" && local.CountryCode != ext.CountryCode {
		return true
	}
	// 组织归属变化也算更新
	if expected := plan.localDepartmentID(ext); expected > 0 {
		if local.DepartmentID == nil || *local.DepartmentID != expected {
			return true
		}
	}
	return false
}

// localDepartmentID 解析外部账号应归属的本地部门 ID（0 表示无法解析/本次新建）
func (p *syncPlan) localDepartmentID(ext syncer.ExternalUser) int64 {
	for _, extDeptID := range ext.DepartmentIDs {
		name, ok := p.deptLocalName[extDeptID]
		if !ok {
			continue
		}
		if id, ok := p.deptIDByName[name]; ok {
			return id
		}
	}
	return 0
}

// guardTripped 同步保护：需要停用的账号达到阈值即取消本次任务（0 = 不限制）。
func guardTripped(plan *syncPlan, guard repository.SyncGuard) (bool, string) {
	if guard.MaxUserDeletes <= 0 {
		return false, ""
	}
	if len(plan.userDisable) >= guard.MaxUserDeletes {
		return true, fmt.Sprintf(
			"%d accounts would be disabled (threshold %d); task cancelled",
			len(plan.userDisable), guard.MaxUserDeletes,
		)
	}
	return false, ""
}

// PreviewPlan 生成试运行预览（不写库）。
func (s *SyncerService) preview(plan *syncPlan, guard repository.SyncGuard) *SyncPreview {
	tripped, message := guardTripped(plan, guard)
	preview := &SyncPreview{
		UserCreate:   len(plan.userCreate),
		UserUpdate:   len(plan.userUpdate),
		UserSkip:     plan.userSkip,
		UserDisable:  len(plan.userDisable),
		DeptCreate:   len(plan.deptCreate),
		DeptUpdate:   len(plan.deptUpdate),
		Warnings:     plan.warnings,
		GuardTripped: tripped,
		GuardMessage: message,
	}
	for i, ext := range plan.userCreate {
		if i >= 5 {
			break
		}
		preview.Samples = append(preview.Samples, sanitizeName(ext.Username))
	}
	for i, portrait := range plan.userUpdate {
		if i >= 5 {
			break
		}
		preview.Samples = append(preview.Samples, portrait.local.Name)
	}
	return preview
}

// applyPlan 落地差异：组织（父先子后）→ 账号（增/改/停用）。
func (s *SyncerService) applyPlan(ctx context.Context, sy *repository.Syncer, plan *syncPlan, work *repository.Syncer) (repository.SyncCounts, []string) {
	var counts repository.SyncCounts
	var errs []string
	appendErr := func(format string, args ...interface{}) {
		if len(errs) >= maxRunErrors {
			return
		}
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	for _, ext := range plan.deptCreate {
		dept, err := s.createDepartment(ctx, sy, plan, ext)
		if err != nil {
			counts.Failed++
			appendErr("department %s: %v", ext.Name, err)
			continue
		}
		plan.deptByName[dept.Name] = dept
		plan.deptIDByName[dept.Name] = dept.ID
		counts.Created++
	}
	for _, ext := range plan.deptUpdate {
		local, ok := plan.deptByName[sanitizeName(firstNonEmptyStr(ext.Name, ext.ID))]
		if !ok {
			continue
		}
		if err := s.moveDepartment(ctx, sy, plan, local); err != nil {
			counts.Failed++
			appendErr("department %s: %v", ext.Name, err)
			continue
		}
		counts.Updated++
	}

	if sy.SyncUsers {
		for _, ext := range plan.userCreate {
			name := sanitizeName(ext.Username)
			user := &repository.User{
				UID:            uuid.NewString(),
				InstitutionID:  sy.InstitutionID,
				Name:           name,
				DisplayName:    firstNonEmptyStr(ext.Name, name),
				Email:          ext.Email,
				Phone:          ext.Phone,
				Avatar:         ext.Avatar,
				Title:          ext.Title,
				Location:       ext.Location,
				CountryCode:    ext.CountryCode,
				EmailVerified:  ext.Email != "",
				RegisterType:   "syncer",
				RegisterSource: sy.Name,
			}
			if id := plan.localDepartmentID(ext); id > 0 {
				user.DepartmentID = &id
			}
			if _, err := s.users.Create(ctx, user); err != nil {
				counts.Failed++
				appendErr("user %s: %v", name, err)
				continue
			}
			counts.Created++
		}
		for i := range plan.userUpdate {
			portrait := plan.userUpdate[i]
			updated, err := s.applyUserUpdate(ctx, plan, &portrait)
			if err != nil {
				counts.Failed++
				appendErr("user %s: %v", portrait.local.Name, err)
				continue
			}
			if updated {
				counts.Updated++
			} else {
				counts.Skipped++
			}
		}
		// 源端缺失 → 停用（软删），不删除本地数据
		for _, local := range plan.userDisable {
			local.IsForbidden = true
			if err := s.users.Update(ctx, local); err != nil {
				counts.Failed++
				appendErr("disable user %s: %v", local.Name, err)
				continue
			}
			counts.Deleted++
		}
	}
	counts.Skipped += plan.userSkip
	return counts, errs
}

func (s *SyncerService) applyUserUpdate(ctx context.Context, plan *syncPlan, portrait *syncPlanPortrait) (bool, error) {
	local, ext := portrait.local, portrait.ext
	changed := false
	if ext.Name != "" && local.DisplayName != ext.Name {
		local.DisplayName, changed = ext.Name, true
	}
	if ext.Email != "" && local.Email != ext.Email {
		local.Email, changed = ext.Email, true
	}
	if ext.Phone != "" && local.Phone != ext.Phone {
		local.Phone, changed = ext.Phone, true
	}
	if ext.Avatar != "" && local.Avatar != ext.Avatar {
		local.Avatar, changed = ext.Avatar, true
	}
	if ext.Title != "" && local.Title != ext.Title {
		local.Title, changed = ext.Title, true
	}
	if ext.Location != "" && local.Location != ext.Location {
		local.Location, changed = ext.Location, true
	}
	if ext.CountryCode != "" && local.CountryCode != ext.CountryCode {
		local.CountryCode, changed = ext.CountryCode, true
	}
	if id := plan.localDepartmentID(ext); id > 0 && (local.DepartmentID == nil || *local.DepartmentID != id) {
		local.DepartmentID, changed = &id, true
	}
	if !changed {
		return false, nil
	}
	if err := s.users.Update(ctx, local); err != nil {
		return false, err
	}
	return true, nil
}

// createDepartment 按外部组织创建本地部门（父节点优先解析「同步目标」或已解析的父部门）
func (s *SyncerService) createDepartment(ctx context.Context, sy *repository.Syncer, plan *syncPlan, ext syncer.ExternalGroup) (*repository.Department, error) {
	name := sanitizeName(firstNonEmptyStr(ext.Name, ext.ID))
	dept := &repository.Department{
		InstitutionID: sy.InstitutionID,
		Name:          name,
		DisplayName:   firstNonEmptyStr(ext.Name, name),
	}
	parent := s.localParentFor(ctx, plan, ext, sy)
	if parent != nil {
		dept.ParentID = &parent.ID
		dept.CodePath = parent.CodePath + "/" + name
		dept.NamePath = parent.NamePath + "/" + dept.DisplayName
		dept.Level = parent.Level + 1
	} else {
		dept.CodePath = name
		dept.NamePath = dept.DisplayName
		dept.Level = 0
	}
	id, err := s.departments.Create(ctx, dept)
	if err != nil {
		return nil, err
	}
	dept.ID = id
	return dept, nil
}

// moveDepartment 把已存在的本地部门移动到外部层级对应的父节点下
func (s *SyncerService) moveDepartment(ctx context.Context, sy *repository.Syncer, plan *syncPlan, local *repository.Department) error {
	parent := s.localParentFor(ctx, plan, syncer.ExternalGroup{ID: local.Name}, sy)
	if parent == nil {
		// 父节点未解析出来（父在本次新建但已落库）→ 重新按当前映射解析
		parent = plan.parentOf(local.Name)
	}
	if parent == nil {
		local.ParentID = nil
		local.CodePath = local.Name
		local.NamePath = local.DisplayName
		local.Level = 0
	} else {
		local.ParentID = &parent.ID
		local.CodePath = parent.CodePath + "/" + local.Name
		local.NamePath = parent.NamePath + "/" + local.DisplayName
		local.Level = parent.Level + 1
	}
	return s.departments.Update(ctx, local)
}

// localParentFor 解析外部组织的父本地部门
func (s *SyncerService) localParentFor(ctx context.Context, plan *syncPlan, ext syncer.ExternalGroup, sy *repository.Syncer) *repository.Department {
	if ext.ParentID != "" {
		if name, ok := plan.deptLocalName[ext.ParentID]; ok {
			if parent, ok := plan.deptByName[name]; ok {
				return parent
			}
		}
	}
	if sy.TargetDepartmentID == nil {
		return nil
	}
	target, err := s.departments.GetByID(ctx, *sy.TargetDepartmentID)
	if err != nil {
		return nil
	}
	return target
}

// parentOf 按本地部门名回溯已解析的父部门（用于父节点在本次新建的场景）
func (p *syncPlan) parentOf(localName string) *repository.Department {
	for _, g := range p.depts {
		if sanitizeName(firstNonEmptyStr(g.Name, g.ID)) != localName {
			continue
		}
		if g.ParentID == "" {
			return nil
		}
		if name, ok := p.deptLocalName[g.ParentID]; ok {
			return p.deptByName[name]
		}
	}
	return nil
}

// DryRun 试运行：拉取 + 差异 + 保护校验，**不写任何数据**。
func (s *SyncerService) DryRun(ctx context.Context, institutionID int64, name string) (*SyncPreview, error) {
	sy, err := s.syncers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("syncer", name)
	}
	connector, err := syncer.Get(sy.Type)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported syncer type")
	}
	plan, err := s.buildPlan(ctx, sy, connector, s.decrypted(sy))
	if err != nil {
		return nil, err
	}
	return s.preview(plan, sy.SyncConfig.Guard), nil
}

// recordRunDetails 写入一次运行记录（含触发来源、耗时与失败明细）。
func (s *SyncerService) recordRunDetails(
	ctx context.Context, sy *repository.Syncer, trigger string, started time.Time,
	status, message string, counts repository.SyncCounts, errs []string,
) {
	now := time.Now()
	_ = s.syncers.UpdateRunResult(ctx, sy.ID, status, message, now, counts)
	countsJSON, _ := json.Marshal(counts)
	errsJSON, _ := json.Marshal(errs)
	if errsJSON == nil {
		errsJSON = []byte("[]")
	}
	_, _ = s.syncers.CreateRun(ctx, &repository.SyncerRun{
		SyncerID: sy.ID, InstitutionID: sy.InstitutionID, Status: status, Trigger: trigger,
		Message: message, Counts: countsJSON, Errors: errsJSON,
		DurationMS: int(now.Sub(started).Milliseconds()),
		StartedAt:  started, FinishedAt: now,
	})
}

// summarizeWarnings 把告警拼进运行消息（最多保留前 3 条）
func summarizeWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	capped := warnings
	if len(capped) > 3 {
		capped = capped[:3]
	}
	return strings.Join(capped, "; ")
}
