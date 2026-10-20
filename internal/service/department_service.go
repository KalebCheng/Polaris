package service

import (
	"context"
	"sort"
	"strconv"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// DepartmentService 部门服务（租户内的部门树）。
// 部门仅用于用户归属与筛选，不参与权限（见 Spec ADR D2）。
type DepartmentService struct {
	depts  repository.DepartmentRepository
	orgs   repository.InstitutionRepository
	users  repository.UserRepository
	audits *AuditRecorder
}

// NewDepartmentService 创建部门服务
func NewDepartmentService(depts repository.DepartmentRepository, orgs repository.InstitutionRepository,
	users repository.UserRepository, audits *AuditRecorder) *DepartmentService {
	return &DepartmentService{depts: depts, orgs: orgs, users: users, audits: audits}
}

// Create 创建部门（校验租户存在、父部门归属一致并维护路径/层级）
func (s *DepartmentService) Create(ctx context.Context, actor string, d *repository.Department) (*repository.Department, error) {
	if d.InstitutionID == 0 || d.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, d.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", d.InstitutionID)
	}
	if _, err := s.depts.GetByName(ctx, d.InstitutionID, d.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "department already exists")
	}

	// 根节点只能由系统创建（组织创建时/迁移 040），不接受手工创建
	d.IsRoot = false
	// 未指定父部门时挂到组织根之下（Spec Q4.5：组织恒定作为部门树的根）
	if d.ParentID == nil {
		if root, err := s.depts.GetRoot(ctx, d.InstitutionID); err == nil {
			d.ParentID = &root.ID
		}
	}
	// 依据父部门补齐 code_path / name_path / level，并标记父节点 has_child
	d.CodePath = d.Name
	d.NamePath = defaultIfEmpty(d.DisplayName, d.Name)
	d.ParentIDChain = []int64{}
	d.Level = 0
	if d.ParentID != nil {
		parent, err := s.depts.GetByID(ctx, *d.ParentID)
		if err != nil || parent.InstitutionID != d.InstitutionID {
			return nil, bizerrors.New(bizerrors.CodeInvalidParams, "invalid parent department")
		}
		d.CodePath = parent.CodePath + "/" + d.Name
		d.NamePath = parent.NamePath + "/" + defaultIfEmpty(d.DisplayName, d.Name)
		d.ParentIDChain = append(append([]int64{}, parent.ParentIDChain...), parent.ID)
		d.Level = len(d.ParentIDChain)
		parent.HasChild = true
		if err := s.depts.Update(ctx, parent); err != nil {
			return nil, err
		}
	}

	id, err := s.depts.Create(ctx, d)
	if err != nil {
		return nil, err
	}
	d.ID = id
	s.audits.Record(ctx, actor, d.InstitutionID, "department.create", d.Name, nil)
	return d, nil
}

// Get 查询部门
func (s *DepartmentService) Get(ctx context.Context, institutionID int64, name string) (*repository.Department, error) {
	d, err := s.depts.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("department", name)
	}
	return d, nil
}

// Update 更新部门：改展示字段，并支持"移动"（变更父级时重算自身与整棵子树的路径/层级）。
func (s *DepartmentService) Update(ctx context.Context, actor string, d *repository.Department) error {
	existing, err := s.depts.GetByName(ctx, d.InstitutionID, d.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("department", d.Name)
	}
	if existing.IsRoot {
		// 组织根是"组织本身"的投影：改名/移动/删除都会让部门树失去根（Spec Q4.5）
		return bizerrors.ErrProtectedObject.WithDetail("department", d.Name)
	}
	// 展示字段按入参，结构性字段以现有为准
	d.ID = existing.ID
	d.CodePath = existing.CodePath
	d.NamePath = defaultIfEmpty(d.DisplayName, d.Name)
	d.ParentIDChain = existing.ParentIDChain
	d.Level = existing.Level
	d.HasChild = existing.HasChild

	parentChanged := !sameParent(d.ParentID, existing.ParentID)
	if !parentChanged {
		d.ParentID = existing.ParentID
		if err := s.depts.Update(ctx, d); err != nil {
			return err
		}
		s.audits.Record(ctx, actor, d.InstitutionID, "department.update", d.Name, nil)
		return nil
	}

	// —— 移动（重挂父级）——
	var parent *repository.Department
	if d.ParentID != nil {
		p, err := s.depts.GetByID(ctx, *d.ParentID)
		if err != nil || p.InstitutionID != d.InstitutionID {
			return bizerrors.New(bizerrors.CodeInvalidParams, "invalid parent department")
		}
		// 禁止把部门挂到自身或其后代下（否则成环）
		if p.ID == existing.ID || isDescendant(p.ParentIDChain, existing.ID) {
			return bizerrors.New(bizerrors.CodeInvalidParams, "cannot move a department under itself or its descendants")
		}
		parent = p
	}

	newCodePath := d.Name
	newNamePath := defaultIfEmpty(d.DisplayName, d.Name)
	newChain := []int64{}
	newLevel := 0
	if parent != nil {
		newCodePath = parent.CodePath + "/" + d.Name
		newNamePath = parent.NamePath + "/" + defaultIfEmpty(d.DisplayName, d.Name)
		newChain = append(append([]int64{}, parent.ParentIDChain...), parent.ID)
		newLevel = len(newChain)
	}
	levelDelta := newLevel - existing.Level

	// 重算自身
	d.CodePath = newCodePath
	d.NamePath = newNamePath
	d.ParentIDChain = newChain
	d.Level = newLevel
	if err := s.depts.Update(ctx, d); err != nil {
		return err
	}

	// 重算整棵子树：code_path 前缀替换 + 层级平移
	all, err := s.depts.List(ctx, repository.ListQuery{InstitutionID: d.InstitutionID, Limit: 10000})
	if err != nil {
		return err
	}
	for _, node := range all {
		if node.ID == existing.ID || !isDescendant(node.ParentIDChain, existing.ID) {
			continue
		}
		moved := *node
		idx := indexOf(node.ParentIDChain, existing.ID)
		suffix := []int64{}
		if idx >= 0 {
			suffix = node.ParentIDChain[idx+1:]
		}
		moved.ParentIDChain = append(append(append([]int64{}, newChain...), existing.ID), suffix...)
		moved.Level = node.Level + levelDelta
		// 仅用于本次响应的展示值；读时由递归 CTE 统一计算。
		oldPrefix := existing.CodePath + "/"
		moved.CodePath = newCodePath + "/" + strings.TrimPrefix(node.CodePath, oldPrefix)
		moved.NamePath = newNamePath + "/" + strings.TrimPrefix(node.NamePath, existing.NamePath+"/")
		if err := s.depts.Update(ctx, &moved); err != nil {
			return err
		}
	}

	// 维护父节点的 has_child 标记（旧父可能已无子）
	s.refreshHasChild(ctx, d.InstitutionID, existing.ParentID)
	s.refreshHasChild(ctx, d.InstitutionID, d.ParentID)

	s.audits.Record(ctx, actor, d.InstitutionID, "department.move", d.Name, nil)
	return nil
}

// refreshHasChild 依据子部门是否存在，修正某父节点的 has_child 标记。
func (s *DepartmentService) refreshHasChild(ctx context.Context, institutionID int64, parentID *int64) {
	if parentID == nil {
		return
	}
	parent, err := s.depts.GetByID(ctx, *parentID)
	if err != nil {
		return
	}
	children, err := s.depts.Count(ctx, repository.ListQuery{InstitutionID: institutionID, Field: "parent_id", Value: strconv.FormatInt(*parentID, 10)})
	if err != nil {
		return
	}
	parent.HasChild = children > 0
	_ = s.depts.Update(ctx, parent)
}

func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// isDescendant 判断 ancestorID 是否在 chain（根→父）中。
func isDescendant(chain []int64, ancestorID int64) bool {
	return indexOf(chain, ancestorID) >= 0
}

func indexOf(chain []int64, id int64) int {
	for i, item := range chain {
		if item == id {
			return i
		}
	}
	return -1
}

// Delete 删除部门（存在子部门或归属用户时禁止）
func (s *DepartmentService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	d, err := s.depts.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("department", name)
	}
	if d.IsRoot {
		return bizerrors.ErrProtectedObject.WithDetail("department", name)
	}
	if d.HasChild {
		return bizerrors.New(bizerrors.CodeConflict, "department still has child departments")
	}
	count, err := s.users.Count(ctx, repository.ListQuery{InstitutionID: institutionID, DepartmentID: d.ID})
	if err == nil && count > 0 {
		return bizerrors.New(bizerrors.CodeConflict, "department still has users")
	}
	if err := s.depts.Delete(ctx, institutionID, name); err != nil {
		return err
	}
	// 删掉最后一个子部门后，父部门的 has_child 标记必须同步复位：
	// Delete 正是靠该标记判断"还有下级"，不复位会让父部门**永远删不掉**（只能先移动一次层级）。
	s.refreshHasChild(ctx, institutionID, d.ParentID)
	s.audits.Record(ctx, actor, institutionID, "department.delete", name, nil)
	return nil
}

// List 列出某租户下的部门
func (s *DepartmentService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Department, int64, error) {
	q := repository.ListQuery{InstitutionID: institutionID, Keyword: keyword, Offset: offset, Limit: limit}
	items, err := s.depts.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.depts.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Members 返回归属某部门的用户（用于右侧详情面板展示成员）。
func (s *DepartmentService) Members(ctx context.Context, institutionID int64, name string) ([]*repository.User, error) {
	d, err := s.depts.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("department", name)
	}
	users, err := s.users.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, DepartmentID: d.ID, Limit: 500,
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

// Reorder 调整部门在其同级内的顺序（direction: "up" | "down"）。
// 以父部门分组，按当前排序（sort_index, name）确定相邻节点并交换二者 sort_index。
func (s *DepartmentService) Reorder(ctx context.Context, actor string, institutionID int64, name, direction string) error {
	d, err := s.depts.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("department", name)
	}
	// 取同租户全部部门，按 (level, sort_index, name) 排序后挑出同级兄弟
	all, err := s.depts.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 10000})
	if err != nil {
		return err
	}
	var siblings []*repository.Department
	for _, node := range all {
		if sameParent(node.ParentID, d.ParentID) && node.Level == d.Level {
			siblings = append(siblings, node)
		}
	}
	// 规范化：为同级节点分配连续的 sort_index（按现有顺序），保证排序稳定可预期
	sort.SliceStable(siblings, func(i, j int) bool {
		if siblings[i].SortIndex != siblings[j].SortIndex {
			return siblings[i].SortIndex < siblings[j].SortIndex
		}
		return siblings[i].Name < siblings[j].Name
	})
	idx := -1
	for i, node := range siblings {
		if node.ID == d.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return bizerrors.ErrNotFound.WithDetail("department", name)
	}
	var swapWith int
	switch direction {
	case "up":
		swapWith = idx - 1
	case "down":
		swapWith = idx + 1
	default:
		return bizerrors.New(bizerrors.CodeInvalidParams, "direction must be up or down")
	}
	if swapWith < 0 || swapWith >= len(siblings) {
		return nil // 已在首/末，视为无操作
	}
	// 交换两节点的 sort_index（先写回规范化序号，再交换）
	for i, node := range siblings {
		switch i {
		case idx:
			target := swapWith
			if node.SortIndex != target {
				if err := s.depts.SetSortIndex(ctx, institutionID, node.Name, target); err != nil {
					return err
				}
			}
		case swapWith:
			target := idx
			if node.SortIndex != target {
				if err := s.depts.SetSortIndex(ctx, institutionID, node.Name, target); err != nil {
					return err
				}
			}
		}
	}
	s.audits.Record(ctx, actor, institutionID, "department.reorder", name, map[string]interface{}{"direction": direction})
	return nil
}

func defaultIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *DepartmentService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.depts.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
