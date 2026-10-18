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
//
// 身份锚定（Spec FR-1）：命中"连接 + 源端 ID"的映射行是**唯一**的判定依据；
// 只有在没有映射行时才回退按本地标识名匹配（兼容接入映射表之前同步进来的存量数据），
// 并在落地阶段补写映射行。这样源端改名不再产生新对象、同步天然幂等。
type syncPlan struct {
	depts []syncer.ExternalGroup
	// deptCreate / deptUpdate 待落地的部门变更
	deptCreate []syncer.ExternalGroup
	deptUpdate []syncer.ExternalGroup
	// deptByName / deptByID 本地部门索引（规划期一次读入，避免逐条查库）
	deptByName map[string]*repository.Department
	deptByID   map[int64]*repository.Department
	// deptLinks 源端组织 ID → 本地部门 ID（含本次新建；落地阶段随创建补齐）
	deptLinks map[string]int64
	// deptNames 源端组织 ID → 本地部门标识（重名时以源端 ID 消歧，保证同组织内唯一）
	deptNames map[string]string
	// deptRootIDs 源端根占位节点（钉钉/企微 1、飞书 0）：不落库，其子节点挂到同步目标/组织根
	deptRootIDs map[string]bool
	// targetID 同步目标部门（0 = 组织根）；rootID 组织根部门
	targetID int64
	rootID   int64

	users       []syncer.ExternalUser
	userCreate  []syncer.ExternalUser
	userUpdate  []syncPlanPortrait
	userSkip    int
	userDisable []*repository.User
	// userByName / userByID 本地账号索引（规划期一次读入）
	userByName map[string]*repository.User
	userByID   map[int64]*repository.User
	// userLinks 源端账号 ID → 本地用户 ID
	userLinks map[string]int64
	// userDepts 本地用户 ID → 本连接此前写入的部门归属（用于判断是否需要重写）
	userDepts map[int64][]int64
	// userTouched 本次同步覆盖到的全部（本地账号, 外部账号）对：落地阶段统一重写部门归属，
	// 因为"多值归属"的真正来源是 user_departments，规划期的单值比较不足以判断收敛
	userTouched []syncPlanPortrait

	// ── 职务（Spec 第六阶段 Q3/F-7：从源端的职务文本派生实体，而不只是写 users.title）──
	// jobTitleByKey 本地职务索引（键为小写名称）；jobTitleByID 主键索引
	jobTitleByKey map[string]*repository.JobTitle
	jobTitleByID  map[int64]*repository.JobTitle
	// jobTitleLinks 源端职务文本（小写）→ 本地 job_title_id（来自映射行）
	jobTitleLinks map[string]int64
	// jobTitleCreate 需要新建的职务（按源端文本去重后的展示名）
	jobTitleCreate []string
	// jobTitleRaw 小写键 → 源端原文（展示名与映射行 external_id 用）
	jobTitleRaw map[string]string
	// userJobTitles 本地用户名 → 该用户在本连接下任职的职务名（落地阶段追加关联）
	userJobTitles map[string][]string

	warnings []string
}

// buildPlan 拉取外部数据并计算差异（不写库）。
func (s *SyncerService) buildPlan(ctx context.Context, sy *repository.Syncer, connector syncer.Connector, work *repository.Syncer) (*syncPlan, error) {
	plan := &syncPlan{
		deptByName:  map[string]*repository.Department{},
		deptByID:    map[int64]*repository.Department{},
		deptLinks:   map[string]int64{},
		deptNames:   map[string]string{},
		deptRootIDs: map[string]bool{},
		userByName:  map[string]*repository.User{},
		userByID:    map[int64]*repository.User{},
		userLinks:   map[string]int64{},
		userDepts:   map[int64][]int64{},
		jobTitleByKey: map[string]*repository.JobTitle{},
		jobTitleByID:  map[int64]*repository.JobTitle{},
		jobTitleLinks: map[string]int64{},
		jobTitleRaw:   map[string]string{},
		userJobTitles: map[string][]string{},
	}

	// 组织根：所有顶层部门（含外部根占位节点的子节点）都挂在它之下（Spec Q4.5）
	if root, err := s.departments.GetRoot(ctx, sy.InstitutionID); err == nil && root != nil {
		plan.rootID = root.ID
		plan.deptByID[root.ID] = root
		plan.deptByName[root.Name] = root
	}
	if sy.TargetDepartmentID != nil {
		if target, err := s.departments.GetByID(ctx, *sy.TargetDepartmentID); err == nil && target.InstitutionID == sy.InstitutionID {
			plan.targetID = target.ID
		}
	}

	existing, err := s.departments.List(ctx, repository.ListQuery{InstitutionID: sy.InstitutionID, Limit: 20000})
	if err != nil {
		return nil, err
	}
	for _, d := range existing {
		plan.deptByName[d.Name] = d
		plan.deptByID[d.ID] = d
	}

	// 外部身份映射（连接 + 源端 ID）——身份锚定的唯一依据
	if links, err := s.links.LoadExternalIndex(ctx, "department", sy.ID); err == nil {
		for externalID, localID := range links {
			plan.deptLinks[externalID] = localID
		}
	}
	if links, err := s.links.LoadExternalIndex(ctx, "user", sy.ID); err == nil {
		for externalID, localID := range links {
			plan.userLinks[externalID] = localID
		}
	}
	if owned, err := s.userDepts.ListBySyncer(ctx, sy.ID); err == nil {
		plan.userDepts = owned
	}
	// 职务：本地字典 + 本连接已有的映射（源端只给职务文本时，用文本作为外部标识）
	if s.jobTitles != nil {
		existingTitles, err := s.jobTitles.List(ctx, repository.ListQuery{InstitutionID: sy.InstitutionID, Limit: 20000})
		if err == nil {
			for _, t := range existingTitles {
				plan.jobTitleByKey[jobTitleKey(t.Name)] = t
				plan.jobTitleByID[t.ID] = t
			}
		}
	}
	if links, err := s.links.LoadExternalIndex(ctx, "job_title", sy.ID); err == nil {
		for externalID, localID := range links {
			plan.jobTitleLinks[jobTitleKey(externalID)] = localID
			plan.jobTitleRaw[jobTitleKey(externalID)] = externalID
		}
	}

	if sy.SyncGroups {
		groups, err := connector.FetchGroups(ctx, work)
		if err != nil {
			// 组织拉取失败不阻断账号同步，但要如实记录告警（试运行与运行记录都会带上）
			plan.warnings = append(plan.warnings, "fetch departments failed: "+err.Error())
			logger.Warn("syncer fetch departments failed", "syncer", sy.Name, "error", err)
		} else {
			plan.depts = groups
			s.planDepartments(plan, groups)
		}
	}

	if sy.SyncUsers {
		localUsers, err := s.users.List(ctx, repository.ListQuery{InstitutionID: sy.InstitutionID, Limit: 50000})
		if err != nil {
			return nil, err
		}
		for _, u := range localUsers {
			plan.userByName[u.Name] = u
			plan.userByID[u.ID] = u
		}
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

// planDepartments 计算组织（部门）树的增改。
//
// 判定顺序：① 映射行命中（连接 + 源端 ID）→ ② 本地同名部门（接入映射表之前的存量数据，落地时补写映射）
// → ③ 新建。**源端根占位节点不落库**，其子节点挂到「同步目标」或组织根之下。
func (s *SyncerService) planDepartments(plan *syncPlan, groups []syncer.ExternalGroup) {
	byExternalID := make(map[string]syncer.ExternalGroup, len(groups))
	for _, g := range groups {
		byExternalID[g.ID] = g
		if g.IsRoot {
			plan.deptRootIDs[g.ID] = true
		}
	}

	visited := map[string]bool{}
	var visit func(g syncer.ExternalGroup) string
	visit = func(g syncer.ExternalGroup) string {
		if g.IsRoot {
			return ""
		}
		name := plan.localDeptName(g)
		plan.deptNames[g.ID] = name
		if visited[g.ID] {
			return name
		}
		visited[g.ID] = true

		local := plan.deptByID[plan.deptLinks[g.ID]]
		if local == nil {
			// 兼容存量：接入映射表之前同步进来的部门只有本地标识名可比对
			local = plan.deptByName[name]
		}
		if local == nil {
			plan.deptCreate = append(plan.deptCreate, g)
			return name
		}
		plan.deptLinks[g.ID] = local.ID
		if parent := s.desiredDeptParent(plan, byExternalID, g); !deptParentMatches(local, parent) {
			plan.deptUpdate = append(plan.deptUpdate, g)
		}
		return name
	}
	for _, g := range groups {
		visit(g)
	}
	// 依赖顺序：父先于子（深度小者先建）
	sort.SliceStable(plan.deptCreate, func(i, j int) bool {
		return s.deptDepth(byExternalID, plan.deptCreate[i]) < s.deptDepth(byExternalID, plan.deptCreate[j])
	})
}

// localDeptName 计算某外部组织在本地应使用的标识名。
//
// 本地 `(institution_id, name)` 自迁移 039 起唯一，而源端不同父级下可能存在同名部门
// （实测：钉钉里两个不同的"市场部"）——因此名称冲突时以源端 ID 消歧，保证结果确定且幂等。
func (p *syncPlan) localDeptName(g syncer.ExternalGroup) string {
	base := sanitizeName(firstNonEmptyStr(g.Name, g.ID))
	if base == "" {
		return "dept-" + sanitizeName(g.ID)
	}
	if existing, ok := p.deptByName[base]; ok && existing.ID != p.deptLinks[g.ID] {
		return sanitizeName(base + "-" + g.ID)
	}
	for otherID, otherName := range p.deptNames {
		if otherName == base && otherID != g.ID {
			return sanitizeName(base + "-" + g.ID)
		}
	}
	return base
}

// desiredDeptParent 解析外部组织在本地应有的父部门（nil = 直接挂在同步目标/组织根之下）
func (s *SyncerService) desiredDeptParent(
	plan *syncPlan, byExternalID map[string]syncer.ExternalGroup, g syncer.ExternalGroup,
) *repository.Department {
	if g.ParentID == "" || plan.deptRootIDs[g.ParentID] {
		return plan.targetOrRoot()
	}
	parent, ok := byExternalID[g.ParentID]
	if !ok || parent.IsRoot {
		return plan.targetOrRoot()
	}
	if id, ok := plan.deptLinks[parent.ID]; ok {
		return plan.deptByID[id]
	}
	// 父节点本次新建：落地阶段（deptLinks 补齐后）再解析
	return nil
}

// targetOrRoot 返回"同步目标"部门；未指定时返回组织根（Spec Q4.5：组织恒定作为部门树的根）
func (p *syncPlan) targetOrRoot() *repository.Department {
	if p.targetID != 0 {
		return p.deptByID[p.targetID]
	}
	return p.deptByID[p.rootID]
}

func (s *SyncerService) deptDepth(byExternalID map[string]syncer.ExternalGroup, g syncer.ExternalGroup) int {
	depth := 0
	seen := map[string]bool{}
	for cur := g; depth < 32; depth++ {
		if cur.ParentID == "" || seen[cur.ID] {
			break
		}
		parent, ok := byExternalID[cur.ParentID]
		if !ok || parent.IsRoot {
			break
		}
		seen[cur.ID] = true
		cur = parent
	}
	return depth
}

// deptParentMatches 判断本地部门的父节点是否与外部层级一致
func deptParentMatches(local *repository.Department, parent *repository.Department) bool {
	if local == nil {
		return false
	}
	if local.ID == parentID(local) {
		// 组织根：其 parent 恒为 nil
		return parent == nil
	}
	if parent == nil {
		return false
	}
	return local.ParentID != nil && *local.ParentID == parent.ID
}

func parentID(d *repository.Department) int64 {
	if d == nil || d.ParentID == nil {
		return 0
	}
	return *d.ParentID
}

// planUsers 计算账号的增/改/跳过，并在开启"源端缺失即停用"时计算需要停用的本地账号。
//
// 身份锚定（Spec FR-1）：先按源端账号 ID 聚合重复条目（一个人在多个部门会返回多条），
// 再按"连接 + 源端 ID"映射命中本地账号；未命中才回退按本地账号名匹配（兼容存量）并补写映射。
func (s *SyncerService) planUsers(ctx context.Context, sy *repository.Syncer, plan *syncPlan, users []syncer.ExternalUser) error {
	merged := mergeExternalUsers(users)
	seenExternal := make(map[string]bool, len(merged))
	seenNames := make(map[string]bool, len(merged))

	for _, ext := range merged {
		name := sanitizeName(ext.Username)
		if name == "" {
			plan.userSkip++
			continue
		}
		seenExternal[ext.ID] = true
		seenNames[name] = true
		plan.collectJobTitle(name, ext.Title)

		local := plan.userByID[plan.userLinks[ext.ID]]
		if local == nil {
			local = plan.userByName[name]
		}
		if local == nil {
			plan.userCreate = append(plan.userCreate, ext)
			continue
		}
		plan.userLinks[ext.ID] = local.ID
		plan.userTouched = append(plan.userTouched, syncPlanPortrait{local: local, ext: ext})
		if userNeedsUpdate(local, ext, plan, local.ID) {
			plan.userUpdate = append(plan.userUpdate, syncPlanPortrait{local: local, ext: ext})
		} else {
			plan.userSkip++
		}
	}

	if !sy.SoftDelete {
		return nil
	}
	disabled := map[int64]bool{}
	// ① 映射行反查：本连接同步过、但源端这次没给的账号
	for externalID, userID := range plan.userLinks {
		if seenExternal[externalID] || disabled[userID] {
			continue
		}
		if local := plan.userByID[userID]; local != nil && !local.IsForbidden && !local.IsDeleted {
			plan.userDisable = append(plan.userDisable, local)
			disabled[userID] = true
		}
	}
	// ② 兜底：还没有映射行的历史同步账号（接入映射表之前同步进来的）
	localUsers, err := s.users.List(ctx, repository.ListQuery{
		InstitutionID: sy.InstitutionID, Field: "register_source", Value: sy.Name, Limit: 50000,
	})
	if err != nil {
		return err
	}
	for _, local := range localUsers {
		if seenNames[local.Name] || local.IsForbidden || local.IsDeleted || disabled[local.ID] {
			continue
		}
		plan.userDisable = append(plan.userDisable, local)
		disabled[local.ID] = true
	}
	return nil
}

// collectJobTitle 从源端账号的职务文本派生本地职务（Spec Q3/F-7）：
// 源端（钉钉 title / 企微 position / 飞书 job_title）**只有文本**，没有实体 ID——
// 因此这里以"职务文本"为外部标识，创建/命中本地 `job_titles` 字典并追加任职关系，
// **不再只把职务写进 users.title 就完事**（那正是"职务全是文本"的根因）。
func (p *syncPlan) collectJobTitle(userName, rawTitle string) {
	title := strings.TrimSpace(rawTitle)
	if title == "" {
		return
	}
	key := jobTitleKey(title)
	p.jobTitleRaw[key] = title
	// 命中顺序：本连接的映射行 → 本地同名职务 → 本次待建
	if _, ok := p.jobTitleLinks[key]; ok {
		if local := p.jobTitleByID[p.jobTitleLinks[key]]; local != nil {
			p.userJobTitles[userName] = dedupeStrings(append(p.userJobTitles[userName], local.Name))
			return
		}
	}
	if local := p.jobTitleByKey[key]; local != nil {
		p.jobTitleLinks[key] = local.ID
		p.userJobTitles[userName] = dedupeStrings(append(p.userJobTitles[userName], local.Name))
		return
	}
	for _, name := range p.jobTitleCreate {
		if jobTitleKey(name) == key {
			p.userJobTitles[userName] = dedupeStrings(append(p.userJobTitles[userName], name))
			return
		}
	}
	p.jobTitleCreate = append(p.jobTitleCreate, title)
	p.userJobTitles[userName] = dedupeStrings(append(p.userJobTitles[userName], title))
}

// jobTitleKey 职务文本的匹配键（大小写与首尾空白不敏感）
func jobTitleKey(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}

// mergeExternalUsers 按源端账号 ID 聚合重复条目：保留首条字段，部门归属取并集。
func mergeExternalUsers(users []syncer.ExternalUser) []syncer.ExternalUser {
	merged := make([]syncer.ExternalUser, 0, len(users))
	index := map[string]int{}
	for _, ext := range users {
		key := ext.ID
		if key == "" {
			key = sanitizeName(ext.Username)
		}
		if i, ok := index[key]; ok {
			merged[i].DepartmentIDs = dedupeStrings(append(merged[i].DepartmentIDs, ext.DepartmentIDs...))
			continue
		}
		ext.DepartmentIDs = dedupeStrings(ext.DepartmentIDs)
		index[key] = len(merged)
		merged = append(merged, ext)
	}
	return merged
}

// resolveDepartmentIDs 把外部部门标识解析成本地主键（保序：主部门取第一个能解析的）
func (p *syncPlan) resolveDepartmentIDs(ext syncer.ExternalUser) []int64 {
	ids := make([]int64, 0, len(ext.DepartmentIDs))
	seen := map[int64]bool{}
	for _, externalID := range ext.DepartmentIDs {
		id, ok := p.deptLinks[externalID]
		if !ok || id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// userNeedsUpdate 判断本地账号是否需要更新（只比较外部提供了值的字段，避免清空本地数据）
func userNeedsUpdate(local *repository.User, ext syncer.ExternalUser, plan *syncPlan, userID int64) bool {
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
	desired := plan.resolveDepartmentIDs(ext)
	if len(desired) > 0 {
		if local.DepartmentID == nil || *local.DepartmentID != desired[0] {
			return true
		}
	}
	if !sameIDSet(desired, plan.userDepts[userID]) {
		return true
	}
	return false
}

// sameIDSet 判断两个 ID 集合是否等价（忽略顺序；nil 与空集合等价）
func sameIDSet(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int64]int, len(a))
	for _, id := range a {
		seen[id]++
	}
	for _, id := range b {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	return true
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

// applyPlan 落地差异：组织（父先子后）→ 账号（增/改/停用）→ 外部身份映射与多部门归属。
func (s *SyncerService) applyPlan(ctx context.Context, sy *repository.Syncer, plan *syncPlan, work *repository.Syncer) (repository.SyncCounts, []string) {
	var counts repository.SyncCounts
	var errs []string
	appendErr := func(format string, args ...interface{}) {
		if len(errs) >= maxRunErrors {
			return
		}
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	linkRaw := func(raw map[string]string) []byte {
		if len(raw) == 0 {
			return []byte("{}")
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			return []byte("{}")
		}
		return encoded
	}

	byExternalID := make(map[string]syncer.ExternalGroup, len(plan.depts))
	for _, g := range plan.depts {
		byExternalID[g.ID] = g
	}

	// ① 组织：新建 → 移动 → 映射行
	for _, ext := range plan.deptCreate {
		dept, err := s.createDepartment(ctx, sy, plan, ext)
		if err != nil {
			counts.Failed++
			appendErr("department %s: %v", ext.Name, err)
			continue
		}
		plan.deptByID[dept.ID] = dept
		plan.deptByName[dept.Name] = dept
		plan.deptLinks[ext.ID] = dept.ID
		counts.Created++
	}
	for _, ext := range plan.deptUpdate {
		local := plan.deptByID[plan.deptLinks[ext.ID]]
		if local == nil {
			continue
		}
		if err := s.moveDepartment(ctx, plan, byExternalID, local, ext); err != nil {
			counts.Failed++
			appendErr("department %s: %v", ext.Name, err)
			continue
		}
		counts.Updated++
	}
	for externalID, localID := range plan.deptLinks {
		if localID == 0 {
			continue
		}
		if err := s.links.Upsert(ctx, "department", sy.InstitutionID, sy.ID, localID, externalID,
			linkRaw(byExternalID[externalID].Raw)); err != nil {
			appendErr("department link %s: %v", externalID, err)
		}
	}

	// ①.5 职务：派生实体（创建缺失的字典项 + 写映射行 + 追加任职关系）
	for _, title := range plan.jobTitleCreate {
		t := &repository.JobTitle{
			InstitutionID: sy.InstitutionID,
			Name:          sanitizeName(title),
			DisplayName:   title,
			IsEnabled:     true,
		}
		if t.Name == "" {
			continue
		}
		if existing := plan.jobTitleByKey[jobTitleKey(t.Name)]; existing != nil {
			plan.jobTitleLinks[jobTitleKey(title)] = existing.ID
			continue
		}
		id, err := s.jobTitles.Create(ctx, t)
		if err != nil {
			counts.Failed++
			appendErr("job title %s: %v", title, err)
			continue
		}
		t.ID = id
		plan.jobTitleByID[id] = t
		plan.jobTitleByKey[jobTitleKey(t.Name)] = t
		plan.jobTitleLinks[jobTitleKey(title)] = id
		counts.Created++
	}
	for key, localID := range plan.jobTitleLinks {
		if localID == 0 {
			continue
		}
		raw := firstNonEmptyStr(plan.jobTitleRaw[key], key)
		if err := s.links.Upsert(ctx, "job_title", sy.InstitutionID, sy.ID, localID, raw,
			linkRaw(map[string]string{"title": raw})); err != nil {
			appendErr("job title link %s: %v", raw, err)
		}
	}

	if sy.SyncUsers {
		// ② 账号：新建 → 更新
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
			if ids := plan.resolveDepartmentIDs(ext); len(ids) > 0 {
				primary := ids[0]
				user.DepartmentID = &primary
			}
			id, err := s.users.Create(ctx, user)
			if err != nil {
				counts.Failed++
				appendErr("user %s: %v", name, err)
				continue
			}
			user.ID = id
			plan.userByID[id] = user
			plan.userByName[name] = user
			plan.userLinks[ext.ID] = id
			plan.userTouched = append(plan.userTouched, syncPlanPortrait{local: user, ext: ext})
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

		// ③ 多部门归属：本次同步覆盖到的账号统一重写（多值关系的真相在 user_departments）
		for _, portrait := range plan.userTouched {
			if portrait.local.ID == 0 {
				continue
			}
			ids := plan.resolveDepartmentIDs(portrait.ext)
			primary := int64(0)
			if len(ids) > 0 {
				primary = ids[0]
			}
			if len(ids) == 0 && len(plan.userDepts[portrait.local.ID]) == 0 {
				continue
			}
			if err := s.userDepts.ReplaceForUser(ctx, sy.InstitutionID, portrait.local.ID, ids, primary, &sy.ID); err != nil {
				counts.Failed++
				appendErr("user departments %s: %v", portrait.local.Name, err)
			}
		}
		// 职务任职关系：只增不删（管理员手工维护的职务不被同步冲掉）
		for userName, titles := range plan.userJobTitles {
			for _, title := range titles {
				local := plan.jobTitleByKey[jobTitleKey(title)]
				if local == nil {
					if id := plan.jobTitleLinks[jobTitleKey(title)]; id != 0 {
						local = plan.jobTitleByID[id]
					}
				}
				if local == nil {
					continue
				}
				if err := s.jobTitles.AddMembers(ctx, local.ID, []string{userName}); err != nil {
					counts.Failed++
					appendErr("job title member %s: %v", title, err)
				}
			}
		}

		for _, portrait := range plan.userTouched {
			if err := s.links.Upsert(ctx, "user", sy.InstitutionID, sy.ID, portrait.local.ID,
				portrait.ext.ID, linkRaw(portrait.ext.Raw)); err != nil {
				appendErr("user link %s: %v", portrait.ext.ID, err)
			}
		}

		// ④ 源端缺失 → 停用（软删），不删除本地数据
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

	// ⑤ 清理源端已消失的映射行（不自动删本地对象：停用由上面的软删逻辑负责）
	seenDepts := make([]string, 0, len(plan.deptLinks))
	for externalID := range plan.deptLinks {
		seenDepts = append(seenDepts, externalID)
	}
	if _, err := s.links.Prune(ctx, "department", sy.ID, seenDepts); err != nil {
		appendErr("prune department links: %v", err)
	}
	if sy.SyncUsers {
		seenUsers := make([]string, 0, len(plan.userLinks))
		for externalID := range plan.userLinks {
			seenUsers = append(seenUsers, externalID)
		}
		if _, err := s.links.Prune(ctx, "user", sy.ID, seenUsers); err != nil {
			appendErr("prune user links: %v", err)
		}
	}

	counts.Skipped += plan.userSkip
	return counts, errs
}

// applyUserUpdate 应用账号字段变更（部门归属由 applyPlan 统一重写，不在这里写）
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
	if !changed {
		return false, nil
	}
	if err := s.users.Update(ctx, local); err != nil {
		return false, err
	}
	return true, nil
}

// createDepartment 按外部组织创建本地部门（父节点优先解析「同步目标」或已解析的父部门）
func (s *SyncerService) createDepartment(
	ctx context.Context, sy *repository.Syncer, plan *syncPlan, ext syncer.ExternalGroup,
) (*repository.Department, error) {
	name, ok := plan.deptNames[ext.ID]
	if !ok || name == "" {
		name = plan.localDeptName(ext)
	}
	dept := &repository.Department{
		InstitutionID: sy.InstitutionID,
		Name:          name,
		DisplayName:   firstNonEmptyStr(ext.Name, name),
	}
	parent := s.applyParentFor(plan, ext)
	if parent != nil {
		dept.ParentID = &parent.ID
		dept.CodePath = parent.CodePath + "/" + name
		dept.NamePath = parent.NamePath + "/" + dept.DisplayName
		dept.Level = parent.Level + 1
		parent.HasChild = true
		if err := s.departments.Update(ctx, parent); err != nil {
			return nil, err
		}
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
func (s *SyncerService) moveDepartment(
	ctx context.Context, plan *syncPlan, byExternalID map[string]syncer.ExternalGroup,
	local *repository.Department, ext syncer.ExternalGroup,
) error {
	parent := s.applyParentFor(plan, ext)
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

// applyParentFor 落地阶段解析外部组织的父本地部门（此时本次新建的部门都已有主键）
func (s *SyncerService) applyParentFor(plan *syncPlan, ext syncer.ExternalGroup) *repository.Department {
	if ext.ParentID == "" || plan.deptRootIDs[ext.ParentID] {
		return plan.targetOrRoot()
	}
	if id, ok := plan.deptLinks[ext.ParentID]; ok {
		if parent := plan.deptByID[id]; parent != nil {
			return parent
		}
	}
	return plan.targetOrRoot()
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
