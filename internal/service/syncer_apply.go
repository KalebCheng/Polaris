package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/syncer"
)

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
	// 批量移动后统一重算祖先链，避免只更新被移动节点、后代仍指向旧链。
	if err := s.departments.RecomputeParentIDChains(ctx, sy.InstitutionID); err != nil {
		appendErr("department chains: %v", err)
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
		DisplayName:   plan.deptDisplayName(ext, name),
	}
	parent := s.applyParentFor(plan, ext)
	if parent != nil {
		dept.ParentID = &parent.ID
		dept.CodePath = parent.CodePath + "/" + name
		dept.NamePath = parent.NamePath + "/" + dept.DisplayName
		dept.ParentIDChain = append(append([]int64{}, parent.ParentIDChain...), parent.ID)
		dept.Level = len(dept.ParentIDChain)
		parent.HasChild = true
		if err := s.departments.Update(ctx, parent); err != nil {
			return nil, err
		}
	} else {
		dept.CodePath = name
		dept.NamePath = dept.DisplayName
		dept.ParentIDChain = []int64{}
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
	local.Name = plan.localDeptName(ext)
	local.DisplayName = plan.deptDisplayName(ext, local.Name)
	parent := s.applyParentFor(plan, ext)
	if parent == nil {
		local.ParentID = nil
		local.CodePath = local.Name
		local.NamePath = local.DisplayName
		local.ParentIDChain = []int64{}
		local.Level = 0
	} else {
		local.ParentID = &parent.ID
		local.CodePath = parent.CodePath + "/" + local.Name
		local.NamePath = parent.NamePath + "/" + local.DisplayName
		local.ParentIDChain = append(append([]int64{}, parent.ParentIDChain...), parent.ID)
		local.Level = len(local.ParentIDChain)
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
