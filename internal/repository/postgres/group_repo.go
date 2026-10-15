package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const groupColumns = `id, institution_id, name, display_name, manager, contact_email, type, parent_id,
	is_top_group, is_enabled, properties, created_at, updated_at`

// GroupRepository 用户组仓库（PostgreSQL 实现）
type GroupRepository struct{ db *dbclient.Client }

// NewGroupRepository 创建用户组仓库
func NewGroupRepository(db *dbclient.Client) *GroupRepository { return &GroupRepository{db: db} }

func scanGroup(row interface{ Scan(...interface{}) error }) (*repository.Group, error) {
	var g repository.Group
	err := row.Scan(&g.ID, &g.InstitutionID, &g.Name, &g.DisplayName, &g.Manager, &g.ContactEmail,
		&g.Type, &g.ParentID, &g.IsTopGroup, &g.IsEnabled, &g.Properties, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// Create 创建用户组
func (r *GroupRepository) Create(ctx context.Context, g *repository.Group) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO groups (institution_id, name, display_name, manager,
		contact_email, type, parent_id, is_top_group, is_enabled, properties)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		g.InstitutionID, g.Name, g.DisplayName, g.Manager, g.ContactEmail, g.Type, g.ParentID,
		g.IsTopGroup, g.IsEnabled, orEmptyMap(g.Properties)).Scan(&id)
	if err != nil {
		return 0, mapError(err, "group not found")
	}
	g.ID = id
	return id, nil
}

// GetByID 按 ID 查询（装配成员）
func (r *GroupRepository) GetByID(ctx context.Context, id int64) (*repository.Group, error) {
	g, err := scanGroup(r.db.QueryRow(ctx, `SELECT `+groupColumns+` FROM groups WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "group not found")
	}
	if err := r.loadMembers(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// GetByName 按组织+名称查询
func (r *GroupRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Group, error) {
	g, err := scanGroup(r.db.QueryRow(ctx, `SELECT `+groupColumns+` FROM groups WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "group not found")
	}
	if err := r.loadMembers(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *GroupRepository) loadMembers(ctx context.Context, g *repository.Group) error {
	rows, err := r.db.Query(ctx, `SELECT "user" FROM group_members WHERE group_id=$1 ORDER BY "user"`, g.ID)
	if err != nil {
		return mapError(err, "")
	}
	defer rows.Close()
	g.Users = []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return mapError(err, "")
		}
		g.Users = append(g.Users, u)
	}
	return mapError(rows.Err(), "")
}

// Update 更新用户组
func (r *GroupRepository) Update(ctx context.Context, g *repository.Group) error {
	return mapError(r.db.Exec(ctx, `UPDATE groups SET display_name=$2, manager=$3, contact_email=$4,
		type=$5, parent_id=$6, is_top_group=$7, is_enabled=$8, properties=$9, updated_at=NOW() WHERE id=$1`,
		g.ID, g.DisplayName, g.Manager, g.ContactEmail, g.Type, g.ParentID, g.IsTopGroup, g.IsEnabled,
		orEmptyMap(g.Properties)), "group not found")
}

// Delete 删除用户组
func (r *GroupRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM groups WHERE id=$1`, id), "group not found")
}

// List 分页列出用户组
func (r *GroupRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Group, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + groupColumns + ` FROM groups` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, g)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计用户组数量
func (r *GroupRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM groups`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// SetMembers 整体替换用户组成员（事务）
func (r *GroupRepository) SetMembers(ctx context.Context, groupID int64, users []string) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM group_members WHERE group_id=$1`, groupID); err != nil {
			return mapError(err, "")
		}
		for _, u := range users {
			if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, "user") VALUES ($1,$2) ON CONFLICT DO NOTHING`, groupID, u); err != nil {
				return mapError(err, "")
			}
		}
		return nil
	})
}

// GroupsForUser 返回用户所属的用户组名
func (r *GroupRepository) GroupsForUser(ctx context.Context, institutionID int64, user string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT g.name FROM groups g
		JOIN group_members gm ON gm.group_id=g.id
		WHERE g.institution_id=$1 AND gm."user"=$2 ORDER BY g.name`, institutionID, user)
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

// SetGroupsForUser 整体替换某用户的用户组归属（事务）。
//
// 只动本组织的组：group_members 没有租户列，故按 group_id 的归属限定删除范围；
// 组名不存在时返回 INVALID_PARAMS（不静默忽略）。
func (r *GroupRepository) SetGroupsForUser(ctx context.Context, institutionID int64, user string, groups []string) error {
	if user == "" {
		return bizerrors.ErrInvalidParams.WithDetail("required", "user")
	}
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM group_members WHERE "user" = $2
			AND group_id IN (SELECT id FROM groups WHERE institution_id = $1)`, institutionID, user); err != nil {
			return mapError(err, "")
		}
		for _, name := range groups {
			if name == "" {
				continue
			}
			res, err := tx.Exec(ctx, `INSERT INTO group_members (group_id, "user")
				SELECT id, $2 FROM groups WHERE institution_id = $1 AND name = $3`, institutionID, user, name)
			if err != nil {
				return mapError(err, "")
			}
			if res.RowsAffected() == 0 {
				return bizerrors.ErrInvalidParams.WithDetail("group", name)
			}
		}
		return nil
	})
}

// MemberCounts 返回组织内各用户组的成员数（列表页展示成员数，避免逐行查询）。
func (r *GroupRepository) MemberCounts(ctx context.Context, institutionID int64) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `SELECT g.name, COUNT(gm."user")
		FROM groups g
		LEFT JOIN group_members gm ON gm.group_id = g.id
		WHERE g.institution_id = $1
		GROUP BY g.name`, institutionID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, mapError(err, "")
		}
		out[name] = count
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.GroupRepository = (*GroupRepository)(nil)
