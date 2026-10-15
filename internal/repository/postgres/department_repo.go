package postgres

import (
	"context"
	"strconv"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// deptColumns departments 表列清单（顺序与 scanDepartment 一致）。
const deptColumns = `id, institution_id, name, display_name, description, parent_id,
	code_path, name_path, level, has_child, sort_index, created_at, updated_at`

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
		&d.CodePath, &d.NamePath, &d.Level, &d.HasChild, &d.SortIndex, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Create 创建部门
func (r *DepartmentRepository) Create(ctx context.Context, d *repository.Department) (int64, error) {
	const q = `INSERT INTO departments (
		institution_id, name, display_name, description, parent_id, code_path, name_path, level, has_child, sort_index)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, q, d.InstitutionID, d.Name, d.DisplayName, d.Description, d.ParentID,
		d.CodePath, d.NamePath, d.Level, d.HasChild, d.SortIndex).Scan(&id)
	if err != nil {
		return 0, mapError(err, "department not found")
	}
	d.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *DepartmentRepository) GetByID(ctx context.Context, id int64) (*repository.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx, `SELECT `+deptColumns+` FROM departments WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "department not found")
	}
	return d, nil
}

// GetByName 按租户 + 名称查询
func (r *DepartmentRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Department, error) {
	d, err := scanDepartment(r.db.QueryRow(ctx,
		`SELECT `+deptColumns+` FROM departments WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "department not found")
	}
	return d, nil
}

// Update 更新部门
func (r *DepartmentRepository) Update(ctx context.Context, d *repository.Department) error {
	const q = `UPDATE departments SET
		display_name=$3, description=$4, parent_id=$5, code_path=$6, name_path=$7,
		level=$8, has_child=$9, sort_index=$10, updated_at=NOW()
		WHERE institution_id=$1 AND name=$2`
	return mapError(r.db.Exec(ctx, q, d.InstitutionID, d.Name, d.DisplayName, d.Description,
		d.ParentID, d.CodePath, d.NamePath, d.Level, d.HasChild, d.SortIndex), "department not found")
}

// Delete 删除部门（子部门通过 parent 外键 ON DELETE SET NULL 处理）
func (r *DepartmentRepository) Delete(ctx context.Context, institutionID int64, name string) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM departments WHERE institution_id=$1 AND name=$2`, institutionID, name), "department not found")
}

// SetSortIndex 仅更新排序序号（同层内排序用；不触碰路径/层级等结构字段）
func (r *DepartmentRepository) SetSortIndex(ctx context.Context, institutionID int64, name string, sortIndex int) error {
	return mapError(r.db.Exec(ctx,
		`UPDATE departments SET sort_index=$3, updated_at=NOW() WHERE institution_id=$1 AND name=$2`,
		institutionID, name, sortIndex), "department not found")
}

// List 列出某租户下的部门（可按关键字或字段过滤）
func (r *DepartmentRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Department, error) {
	sql := `SELECT ` + deptColumns + ` FROM departments WHERE institution_id=$1`
	args := []interface{}{q.InstitutionID}
	if q.Value != "" && q.Field != "" {
		args = append(args, q.Value)
		sql += ` AND ` + q.Field + ` = $` + strconv.Itoa(len(args))
	} else if q.Keyword != "" {
		args = append(args, "%"+q.Keyword+"%")
		p := "$" + strconv.Itoa(len(args))
		sql += ` AND (name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`
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

// ExpandMembersWithDescendants 展开"这些部门 + 其全部子部门"内的用户名集合。
//
// 用 parent_id 递归而非 code_path 前缀匹配：两者等价，但前者不依赖路径字符串格式，
// 且复用与用户组展开同一套递归写法（部门移动时无需特殊处理）。
func (r *DepartmentRepository) ExpandMembersWithDescendants(ctx context.Context, institutionID int64, names []string) ([]string, error) {
	if len(names) == 0 {
		return []string{}, nil
	}
	rows, err := r.db.Query(ctx, `WITH RECURSIVE tree AS (
			SELECT d.id FROM departments d WHERE d.institution_id = $1 AND d.name = ANY($2)
			UNION
			SELECT c.id FROM departments c JOIN tree t ON c.parent_id = t.id
		)
		SELECT DISTINCT u.name FROM users u JOIN tree t ON u.department_id = t.id
		WHERE u.institution_id = $1
		ORDER BY 1`, institutionID, names)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, name)
	}
	return out, mapError(rows.Err(), "")
}

// Children 返回某节点的**直接子部门**（parentID 为 nil 时取顶层），并返回该层总数。
//
// 用途：应用授权弹窗的部门树按层懒加载——顶层先取一页，展开某节点时再取它的孩子，
// 避免一次性把整棵部门树（大组织可达数百节点）拉到前端。
func (r *DepartmentRepository) Children(ctx context.Context, institutionID int64, parentID *int64, offset, limit int) ([]*repository.Department, int64, error) {
	cond := "WHERE institution_id=$1 AND parent_id IS NULL"
	args := []interface{}{institutionID}
	if parentID != nil {
		cond = "WHERE institution_id=$1 AND parent_id=$2"
		args = append(args, *parentID)
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM departments `+cond, args...).Scan(&total); err != nil {
		return nil, 0, mapError(err, "")
	}
	queryArgs := append(append([]interface{}{}, args...), limit, offset)
	sql := `SELECT ` + deptColumns + ` FROM departments ` + cond +
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

// ExistingNames 返回 names 中确实存在于本组织的部门标识集合（授权名单逐项校验用）。
func (r *DepartmentRepository) ExistingNames(ctx context.Context, institutionID int64, names []string) (map[string]bool, error) {
	return existingNames(ctx, r.db, "departments", institutionID, names)
}
