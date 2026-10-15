package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const roleColumns = `id, institution_id, name, display_name, description, is_enabled, is_system, created_at, updated_at`

// RoleRepository 角色仓库（PostgreSQL 实现）
type RoleRepository struct{ db *dbclient.Client }

// NewRoleRepository 创建角色仓库
func NewRoleRepository(db *dbclient.Client) *RoleRepository { return &RoleRepository{db: db} }

func scanRole(row interface{ Scan(...interface{}) error }) (*repository.Role, error) {
	var r repository.Role
	err := row.Scan(&r.ID, &r.InstitutionID, &r.Name, &r.DisplayName, &r.Description, &r.IsEnabled,
		&r.IsSystem, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Create 创建角色
func (r *RoleRepository) Create(ctx context.Context, role *repository.Role) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO roles (institution_id, name, display_name, description, is_enabled, is_system)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		role.InstitutionID, role.Name, role.DisplayName, role.Description, role.IsEnabled, role.IsSystem).Scan(&id)
	if err != nil {
		return 0, mapError(err, "role not found")
	}
	role.ID = id
	return id, nil
}

// GetByID 按 ID 查询（装配成员关系）
func (r *RoleRepository) GetByID(ctx context.Context, id int64) (*repository.Role, error) {
	role, err := scanRole(r.db.QueryRow(ctx, `SELECT `+roleColumns+` FROM roles WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "role not found")
	}
	if err := r.loadRelations(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// GetByName 按组织+名称查询
func (r *RoleRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Role, error) {
	role, err := scanRole(r.db.QueryRow(ctx, `SELECT `+roleColumns+` FROM roles WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "role not found")
	}
	if err := r.loadRelations(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// loadRelations 装配角色关联（用户/用户组/子角色/权限）
func (r *RoleRepository) loadRelations(ctx context.Context, role *repository.Role) error {
	var err error
	role.Users, err = r.strings(ctx, `SELECT "user" FROM role_users WHERE role_id=$1 ORDER BY "user"`, role.ID)
	if err != nil {
		return err
	}
	role.Groups, err = r.strings(ctx, `SELECT "group" FROM role_groups WHERE role_id=$1 ORDER BY "group"`, role.ID)
	if err != nil {
		return err
	}
	role.Roles, err = r.strings(ctx, `SELECT child_role FROM role_roles WHERE role_id=$1 ORDER BY child_role`, role.ID)
	if err != nil {
		return err
	}
	return nil
}

func (r *RoleRepository) strings(ctx context.Context, sql string, args ...interface{}) ([]string, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, s)
	}
	return out, mapError(rows.Err(), "")
}

// Update 更新角色基础信息
func (r *RoleRepository) Update(ctx context.Context, role *repository.Role) error {
	return mapError(r.db.Exec(ctx, `UPDATE roles SET display_name=$2, description=$3, is_enabled=$4, updated_at=NOW() WHERE id=$1`,
		role.ID, role.DisplayName, role.Description, role.IsEnabled), "role not found")
}

// Delete 删除角色
func (r *RoleRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM roles WHERE id=$1`, id), "role not found")
}

// List 分页列出角色
func (r *RoleRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Role, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + roleColumns + ` FROM roles` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, role)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计角色数量
func (r *RoleRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM roles`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// SetMembers 整体替换角色的成员/子角色/权限（事务）。
func (r *RoleRepository) SetMembers(ctx context.Context, roleID int64, rel *repository.RoleRelations) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM role_users WHERE role_id=$1`, roleID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_groups WHERE role_id=$1`, roleID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_roles WHERE role_id=$1`, roleID); err != nil {
			return mapError(err, "")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, roleID); err != nil {
			return mapError(err, "")
		}
		for _, u := range rel.Users {
			if _, err := tx.Exec(ctx, `INSERT INTO role_users (role_id, "user") VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, u); err != nil {
				return mapError(err, "")
			}
		}
		for _, g := range rel.Groups {
			if _, err := tx.Exec(ctx, `INSERT INTO role_groups (role_id, "group") VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, g); err != nil {
				return mapError(err, "")
			}
		}
		for _, c := range rel.Roles {
			if _, err := tx.Exec(ctx, `INSERT INTO role_roles (role_id, child_role) VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, c); err != nil {
				return mapError(err, "")
			}
		}
		for _, p := range rel.Permissions {
			if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, p); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

const rolesForUserSQL = `
WITH RECURSIVE user_groups(name) AS (
    SELECT g.name FROM groups g
    JOIN group_members gm ON gm.group_id = g.id
    WHERE g.institution_id = $1 AND gm."user" = $2
),
base_roles(name) AS (
    SELECT r.name FROM roles r
    JOIN role_users ru ON ru.role_id = r.id
    WHERE r.institution_id = $1 AND ru."user" = $2
    UNION
    SELECT r.name FROM roles r
    JOIN role_groups rg ON rg."group" = r.name
    JOIN user_groups ug ON ug.name = rg."group"
    WHERE r.institution_id = $1
),
role_tree(name) AS (
    SELECT name FROM base_roles
    UNION
    SELECT rr.child_role FROM role_roles rr
    JOIN roles r ON r.id = rr.role_id
    JOIN role_tree rt ON rt.name = r.name
    WHERE r.institution_id = $1
)
SELECT name FROM role_tree`

// RolesForUser 返回用户直接/经用户组/经嵌套获得的角色名。
func (r *RoleRepository) RolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return r.strings(ctx, rolesForUserSQL, institutionID, user)
}

const permissionsForUserSQL = `
WITH RECURSIVE user_groups(name) AS (
    SELECT g.name FROM groups g
    JOIN group_members gm ON gm.group_id = g.id
    WHERE g.institution_id = $1 AND gm."user" = $2
),
base_roles(name) AS (
    SELECT r.name FROM roles r
    JOIN role_users ru ON ru.role_id = r.id
    WHERE r.institution_id = $1 AND ru."user" = $2
    UNION
    SELECT r.name FROM roles r
    JOIN role_groups rg ON rg."group" = r.name
    JOIN user_groups ug ON ug.name = rg."group"
    WHERE r.institution_id = $1
),
role_tree(name) AS (
    SELECT name FROM base_roles
    UNION
    SELECT rr.child_role FROM role_roles rr
    JOIN roles r ON r.id = rr.role_id
    JOIN role_tree rt ON rt.name = r.name
    WHERE r.institution_id = $1
)
SELECT p.name, p.resource_type, p.resources, p.actions, p.effect
FROM permissions p
WHERE p.institution_id = $1 AND p.is_enabled = TRUE
  AND (p.expire_time IS NULL OR p.expire_time > NOW())
  AND (
    p.name IN (SELECT rp.permission FROM role_permissions rp
               JOIN roles r ON r.id = rp.role_id
               WHERE r.institution_id = $1 AND r.name IN (SELECT name FROM role_tree))
    OR p.id IN (SELECT pu.permission_id FROM permission_users pu WHERE pu."user" = $2)
    OR p.id IN (SELECT pg.permission_id FROM permission_groups pg
                WHERE pg."group" IN (SELECT name FROM user_groups))
  )`

// PermissionsForUser 返回用户的有效权限集合（角色 / 用户组 / 直接绑定 三种来源的并集）。
func (r *RoleRepository) PermissionsForUser(ctx context.Context, institutionID int64, user string) ([]*repository.Permission, error) {
	rows, err := r.db.Query(ctx, permissionsForUserSQL, institutionID, user)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Permission
	for rows.Next() {
		var p repository.Permission
		p.InstitutionID = institutionID
		if err := rows.Scan(&p.Name, &p.ResourceType, &p.Resources, &p.Actions, &p.Effect); err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, &p)
	}
	return result, mapError(rows.Err(), "")
}

// directRolesForUserSQL 用户**直接**归属的角色（不含经用户组、不含嵌套子角色）。
const directRolesForUserSQL = `
SELECT r.name FROM roles r
JOIN role_users ru ON ru.role_id = r.id
WHERE r.institution_id = $1 AND ru."user" = $2
ORDER BY r.name`

// DirectRolesForUser 返回用户的直接角色（"用户详情-授权"标签的整体替换基线）。
func (r *RoleRepository) DirectRolesForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	return r.strings(ctx, directRolesForUserSQL, institutionID, user)
}

// SetDirectRoles 整体替换用户的直接角色（事务）。
//
// 只动本组织角色：role_users 没有租户列，因此按 role_id 归属限定删除范围，
// 避免跨组织同名用户被连带清空。角色名不存在时返回 INVALID_PARAMS（不做静默忽略）。
func (r *RoleRepository) SetDirectRoles(ctx context.Context, institutionID int64, user string, roles []string) error {
	if user == "" {
		return bizerrors.ErrInvalidParams.WithDetail("required", "user")
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM role_users WHERE "user" = $2
			AND role_id IN (SELECT id FROM roles WHERE institution_id = $1)`, institutionID, user); err != nil {
			return mapError(err, "")
		}
		for _, role := range roles {
			if role == "" {
				continue
			}
			res, err := tx.Exec(ctx, `INSERT INTO role_users (role_id, "user")
				SELECT id, $2 FROM roles WHERE institution_id = $1 AND name = $3`, institutionID, user, role)
			if err != nil {
				return mapError(err, "")
			}
			if res.RowsAffected() == 0 {
				return bizerrors.ErrInvalidParams.WithDetail("role", role)
			}
		}
		return nil
	})
}

// permissionBindingsForUserSQL 有效权限的**来源**明细（与 permissionsForUserSQL 同一套继承语义）。
const permissionBindingsForUserSQL = `
WITH RECURSIVE user_groups(name) AS (
    SELECT g.name FROM groups g
    JOIN group_members gm ON gm.group_id = g.id
    WHERE g.institution_id = $1 AND gm."user" = $2
),
base_roles(name) AS (
    SELECT r.name FROM roles r
    JOIN role_users ru ON ru.role_id = r.id
    WHERE r.institution_id = $1 AND ru."user" = $2
    UNION
    SELECT r.name FROM roles r
    JOIN role_groups rg ON rg."group" = r.name
    JOIN user_groups ug ON ug.name = rg."group"
    WHERE r.institution_id = $1
),
role_tree(name) AS (
    SELECT name FROM base_roles
    UNION
    SELECT rr.child_role FROM role_roles rr
    JOIN roles r ON r.id = rr.role_id
    JOIN role_tree rt ON rt.name = r.name
    WHERE r.institution_id = $1
)
SELECT p.name, 'user' AS source
FROM permissions p
JOIN permission_users pu ON pu.permission_id = p.id
WHERE p.institution_id = $1 AND pu."user" = $2
UNION ALL
SELECT p.name, 'role:' || r.name AS source
FROM permissions p
JOIN role_permissions rp ON rp.permission = p.name
JOIN roles r ON r.id = rp.role_id
WHERE p.institution_id = $1 AND r.institution_id = $1 AND r.name IN (SELECT name FROM role_tree)
UNION ALL
SELECT p.name, 'group:' || pg."group" AS source
FROM permissions p
JOIN permission_groups pg ON pg.permission_id = p.id
WHERE p.institution_id = $1 AND pg."group" IN (SELECT name FROM user_groups)
ORDER BY 1, 2`

// PermissionBindingsForUser 返回"权限名 → 来源"明细（user / role:<角色> / group:<用户组>）。
func (r *RoleRepository) PermissionBindingsForUser(ctx context.Context, institutionID int64, user string) ([]repository.PermissionBinding, error) {
	rows, err := r.db.Query(ctx, permissionBindingsForUserSQL, institutionID, user)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []repository.PermissionBinding{}
	for rows.Next() {
		var b repository.PermissionBinding
		if err := rows.Scan(&b.Permission, &b.Source); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, b)
	}
	return out, mapError(rows.Err(), "")
}

// rolesForGroupSQL 用户组关联的角色（含经嵌套子角色继承）。
const rolesForGroupSQL = `
WITH RECURSIVE base_roles(name) AS (
    SELECT r.name FROM roles r
    JOIN role_groups rg ON rg.role_id = r.id
    WHERE r.institution_id = $1 AND rg."group" = $2
),
role_tree(name) AS (
    SELECT name FROM base_roles
    UNION
    SELECT rr.child_role FROM role_roles rr
    JOIN roles r ON r.id = rr.role_id
    JOIN role_tree rt ON rt.name = r.name
    WHERE r.institution_id = $1
)
SELECT name FROM role_tree ORDER BY name`

// RolesForGroup 返回用户组关联的角色名。
func (r *RoleRepository) RolesForGroup(ctx context.Context, institutionID int64, group string) ([]string, error) {
	return r.strings(ctx, rolesForGroupSQL, institutionID, group)
}

// permissionsForGroupSQL 用户组经角色获得的有效权限。
const permissionsForGroupSQL = `
WITH RECURSIVE base_roles(name) AS (
    SELECT r.name FROM roles r
    JOIN role_groups rg ON rg.role_id = r.id
    WHERE r.institution_id = $1 AND rg."group" = $2
),
role_tree(name) AS (
    SELECT name FROM base_roles
    UNION
    SELECT rr.child_role FROM role_roles rr
    JOIN roles r ON r.id = rr.role_id
    JOIN role_tree rt ON rt.name = r.name
    WHERE r.institution_id = $1
)
SELECT p.name, p.resource_type, p.resources, p.actions, p.effect
FROM permissions p
WHERE p.institution_id = $1 AND p.is_enabled = TRUE
  AND (p.expire_time IS NULL OR p.expire_time > NOW())
  AND p.name IN (SELECT rp.permission FROM role_permissions rp
                 JOIN roles r ON r.id = rp.role_id
                 WHERE r.institution_id = $1 AND r.name IN (SELECT name FROM role_tree))`

// PermissionsForGroup 返回用户组经角色获得的权限（去重）。
func (r *RoleRepository) PermissionsForGroup(ctx context.Context, institutionID int64, group string) ([]*repository.Permission, error) {
	rows, err := r.db.Query(ctx, permissionsForGroupSQL, institutionID, group)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []*repository.Permission{}
	for rows.Next() {
		var p repository.Permission
		p.InstitutionID = institutionID
		if err := rows.Scan(&p.Name, &p.ResourceType, &p.Resources, &p.Actions, &p.Effect); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, &p)
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.RoleRepository = (*RoleRepository)(nil)
