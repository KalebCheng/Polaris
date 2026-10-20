package postgres

import (
	"context"
	"strconv"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// deptSelect 读时用递归 CTE 计算 code_path / name_path / level，数据库不存这三列。
const deptSelect = `WITH RECURSIVE paths AS (
	SELECT id, parent_id, name, display_name, 0 AS level,
	       name::TEXT AS code_path,
	       COALESCE(NULLIF(display_name, ''), name)::TEXT AS name_path
	FROM departments WHERE parent_id IS NULL
	UNION ALL
	SELECT d.id, d.parent_id, d.name, d.display_name, p.level + 1,
	       p.code_path || '/' || d.name,
	       p.name_path || '/' || COALESCE(NULLIF(d.display_name, ''), d.name)
	FROM departments d JOIN paths p ON d.parent_id = p.id
)
SELECT d.id, d.institution_id, d.name, d.display_name, d.description, d.parent_id,
       d.parent_id_chain, p.code_path, p.name_path, p.level, d.has_child,
       d.sort_index, d.is_root, d.created_at, d.updated_at
FROM departments d JOIN paths p ON p.id = d.id`

// DepartmentRepository 部门仓库（PostgreSQL 实现）
type DepartmentRepository struct {
	db *dbclient.Client
}

// NewDepartmentRepository 创建部门仓库
func NewDepartmentRepository(db *dbclient.Client) *DepartmentRepository {
	return &DepartmentRepository{db: db}
}

func scanDepartment(row interface{ Scan(...interface{}) error }) (*repository.Department, error) {
	var d repository.Department
	err := row.Scan(&d.ID, &d.InstitutionID, &d.Name, &d.DisplayName, &d.Description, &d.ParentID,
		&d.ParentIDChain, &d.CodePath, &d.NamePath, &d.Level, &d.HasChild, &d.SortIndex, &d.IsRoot,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Create 创建部门
func (r *DepartmentRepository) Create(ctx context.Context, d *repository.Department) (int64, error) {
	chain := d.ParentIDChain
	if chain == nil {
		chain = []int64{}
	}
	d.ParentIDChain = chain
	const q = `INSERT INTO departments (
		institution_id, name, display_name, description, parent_id, parent_id_chain, has_child, sort_index, is_root)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, q, d.InstitutionID, d.Name, d.DisplayName, d.Description, d.ParentID,
		chain, d.HasChild, d.SortIndex, d.IsRoot).Scan(&id)
	if err != nil {
		return 0, mapError(err, "department not found")
	}
	d.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *DepartmentRepository) GetByID(ctx context.Context, id int64) (*repository.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx, deptSelect+` WHERE d.id=$1`, id))
	if err != nil {
		return nil, mapError(err, "department not found")
	}
	return d, nil
}

// GetByName 按租户 + 名称查询
func (r *DepartmentRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx,
		deptSelect+` WHERE d.institution_id=$1 AND d.name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "department not found")
	}
	return d, nil
}

// Update 更新部门
func (r *DepartmentRepository) Update(ctx context.Context, d *repository.Department) error {
	chain := d.ParentIDChain
	if chain == nil {
		chain = []int64{}
	}
	const q = `UPDATE departments SET
		display_name=$3, description=$4, parent_id=$5, parent_id_chain=$6,
		has_child=$7, sort_index=$8, updated_at=NOW()
		WHERE institution_id=$1 AND name=$2`
	return mapError(r.db.Exec(ctx, q, d.InstitutionID, d.Name, d.DisplayName, d.Description,
		d.ParentID, chain, d.HasChild, d.SortIndex), "department not found")
}

// Delete 删除部门（子部门通过 parent 外键 ON DELETE SET NULL 处理）
func (r *DepartmentRepository) Delete(ctx context.Context, institutionID int64, name string) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM departments WHERE institution_id=$1 AND name=$2`, institutionID, name), "department not found")
}

// GetRoot 返回组织根部门（每个组织恰好一行 is_root = TRUE；见迁移 040）
func (r *DepartmentRepository) GetRoot(ctx context.Context, institutionID int64) (*repository.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx,
		deptSelect+` WHERE d.institution_id=$1 AND d.is_root`, institutionID))
	if err != nil {
		return nil, mapError(err, "department not found")
	}
	return d, nil
}

// SetSortIndex 仅更新排序序号（同层内排序用；不触碰路径/层级等结构字段）
func (r *DepartmentRepository) SetSortIndex(ctx context.Context, institutionID int64, name string, sortIndex int) error {
	return mapError(r.db.Exec(ctx,
		`UPDATE departments SET sort_index=$3, updated_at=NOW() WHERE institution_id=$1 AND name=$2`,
		institutionID, name, sortIndex), "department not found")
}

// RecomputeParentIDChains 按 parent_id 重算祖先链（根→父，不含自身）。
func (r *DepartmentRepository) RecomputeParentIDChains(ctx context.Context, institutionID int64) error {
	err := r.db.Exec(ctx, `WITH RECURSIVE tree AS (
		SELECT id, parent_id, ARRAY[]::BIGINT[] AS chain
		FROM departments WHERE institution_id=$1 AND parent_id IS NULL
		UNION ALL
		SELECT d.id, d.parent_id, t.chain || d.parent_id
		FROM departments d JOIN tree t ON d.parent_id=t.id
	)
	UPDATE departments d SET parent_id_chain=t.chain FROM tree t WHERE t.id=d.id`, institutionID)
	return mapError(err, "")
}

// List 列出某租户下的部门（可按关键字或字段过滤）
func (r *DepartmentRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Department, error) {
	sql := deptSelect + ` WHERE d.institution_id=$1`
	args := []interface{}{q.InstitutionID}
	if q.Value != "" && q.Field != "" {
		args = append(args, q.Value)
		sql += ` AND d.` + q.Field + ` = $` + strconv.Itoa(len(args))
	} else if q.Keyword != "" {
		args = append(args, "%"+q.Keyword+"%")
		p := "$" + strconv.Itoa(len(args))
		sql += ` AND (d.name ILIKE ` + p + ` OR d.display_name ILIKE ` + p + `)`
	}
	sql += ` ORDER BY level ASC, sort_index ASC, name ASC`
	if q.Limit > 0 {
		args = append(args, q.Limit, q.Offset)
		sql += ` LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
	}
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()

	var result []*repository.Department
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, d)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计某租户下的部门数量（可按字段过滤）
func (r *DepartmentRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	sql := `SELECT COUNT(*) FROM departments WHERE institution_id=$1`
	args := []interface{}{q.InstitutionID}
	if q.Value != "" && q.Field != "" {
		args = append(args, q.Value)
		sql += ` AND ` + q.Field + ` = $` + strconv.Itoa(len(args))
	} else if q.Keyword != "" {
		args = append(args, "%"+q.Keyword+"%")
		sql += ` AND (name ILIKE $2 OR display_name ILIKE $2)`
	}
	var n int64
	err := r.db.QueryRow(ctx, sql, args...).Scan(&n)
	return n, mapError(err, "")
}

// Children 返回某节点的**直接子部门**（parentID 为 nil 时取顶层），并返回该层总数。
//
// 用途：应用授权弹窗的部门树按层懒加载——顶层先取一页，展开某节点时再取它的孩子，
// 避免一次性把整棵部门树（大组织可达数百节点）拉到前端。
func (r *DepartmentRepository) Children(ctx context.Context, institutionID int64, parentID *int64, offset, limit int) ([]*repository.Department, int64, error) {
	cond := "WHERE d.institution_id=$1 AND d.parent_id IS NULL"
	args := []interface{}{institutionID}
	if parentID != nil {
		cond = "WHERE d.institution_id=$1 AND d.parent_id=$2"
		args = append(args, *parentID)
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM departments d `+cond, args...).Scan(&total); err != nil {
		return nil, 0, mapError(err, "")
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	sql := deptSelect + ` ` + cond +
		` ORDER BY sort_index ASC, name ASC LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	rows, err := r.db.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, 0, mapError(err, "")
	}
	defer rows.Close()
	result := []*repository.Department{}
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			return nil, 0, mapError(err, "")
		}
		result = append(result, d)
	}
	return result, total, mapError(rows.Err(), "")
}

// ExistingIDs returns department IDs that belong to the institution.
func (r *DepartmentRepository) ExistingIDs(ctx context.Context, institutionID int64, ids []int64) (map[int64]bool, error) {
	return existingIDs(ctx, r.db, "departments", institutionID, ids)
}
