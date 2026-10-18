package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// UserDepartmentRepository 用户 ↔ 部门多值归属仓库（PostgreSQL 实现；表 user_departments）
type UserDepartmentRepository struct{ db *dbclient.Client }

// NewUserDepartmentRepository 创建仓库
func NewUserDepartmentRepository(db *dbclient.Client) *UserDepartmentRepository {
	return &UserDepartmentRepository{db: db}
}

// ReplaceForUser 整体替换某用户的部门归属（事务）：
//   - syncerID 非空时只替换"该连接写入的归属"，管理员手工维护的归属不受同步影响；
//   - 同步 users.department_id = 主部门（多值关系的真相在 user_departments，主部门列是派生的便利视图）。
func (r *UserDepartmentRepository) ReplaceForUser(
	ctx context.Context, institutionID, userID int64, departmentIDs []int64, primaryID int64, syncerID *int64,
) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if syncerID == nil {
			if _, err := tx.Exec(ctx, `DELETE FROM user_departments WHERE user_id=$1`, userID); err != nil {
				return mapError(err, "")
			}
		} else {
			if _, err := tx.Exec(ctx, `DELETE FROM user_departments WHERE user_id=$1 AND syncer_id=$2`, userID, *syncerID); err != nil {
				return mapError(err, "")
			}
		}
		for _, deptID := range departmentIDs {
			if deptID == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO user_departments
				(institution_id, user_id, department_id, is_primary, syncer_id)
				VALUES ($1,$2,$3,$4,$5) ON CONFLICT (user_id, department_id)
				DO UPDATE SET syncer_id = EXCLUDED.syncer_id`, institutionID, userID, deptID, deptID == primaryID, syncerID); err != nil {
				return mapError(err, "")
			}
		}
		var primary *int64
		if primaryID > 0 {
			primary = &primaryID
			if _, err := tx.Exec(ctx, `UPDATE user_departments SET is_primary = TRUE WHERE user_id=$1 AND department_id=$2`, userID, primaryID); err != nil {
				return mapError(err, "")
			}
			if _, err := tx.Exec(ctx, `UPDATE user_departments SET is_primary = FALSE WHERE user_id=$1 AND department_id<>$2`, userID, primaryID); err != nil {
				return mapError(err, "")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET department_id=$2, updated_at=NOW() WHERE id=$1`, userID, primary); err != nil {
			return mapError(err, "user not found")
		}
		return nil
	})
}

// DepartmentIDsForUser 返回用户的全部部门归属（主部门优先）
func (r *UserDepartmentRepository) DepartmentIDsForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT department_id FROM user_departments
		WHERE user_id=$1 ORDER BY is_primary DESC, department_id`, userID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, id)
	}
	return out, mapError(rows.Err(), "")
}

// ListForUser 返回用户的部门归属明细（含主部门标记与来源连接）
func (r *UserDepartmentRepository) ListForUser(ctx context.Context, userID int64) ([]*repository.UserDepartment, error) {
	return queryEntities[repository.UserDepartment](ctx, r.db, "user_departments", "WHERE user_id=$1", []interface{}{userID}, "is_primary DESC, department_id", 0, 0)
}

// ListBySyncer 返回某连接写入的全部归属（user_id → department_id 列表）
func (r *UserDepartmentRepository) ListBySyncer(ctx context.Context, syncerID int64) (map[int64][]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id, department_id FROM user_departments
		WHERE syncer_id=$1 ORDER BY user_id, department_id`, syncerID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var userID, deptID int64
		if err := rows.Scan(&userID, &deptID); err != nil {
			return nil, mapError(err, "")
		}
		out[userID] = append(out[userID], deptID)
	}
	return out, mapError(rows.Err(), "")
}

// UserIDsOfDepartments 返回"这些部门 + 其全部子部门"内的用户 id 集合（应用访问影响面计算用；部门保留层级语义）
func (r *UserDepartmentRepository) UserIDsOfDepartments(ctx context.Context, institutionID int64, departmentIDs []int64) ([]int64, error) {
	if len(departmentIDs) == 0 {
		return []int64{}, nil
	}
	rows, err := r.db.Query(ctx, `WITH RECURSIVE tree AS (
			SELECT id FROM departments WHERE institution_id=$1 AND id = ANY($2)
			UNION
			SELECT c.id FROM departments c JOIN tree t ON c.parent_id = t.id
		)
		SELECT DISTINCT ud.user_id FROM user_departments ud JOIN tree t ON ud.department_id = t.id ORDER BY 1`,
		institutionID, departmentIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, id)
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.UserDepartmentRepository = (*UserDepartmentRepository)(nil)
