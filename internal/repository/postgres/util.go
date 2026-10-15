// Package postgres 提供基于 PostgreSQL（pgx v5）的仓库实现。
package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	bizerrors "polaris/internal/errors"
	"polaris/pkg/dbclient"
)

// mapError 将底层数据库错误转换为统一业务错误（隐藏底层细节，仅保留语义）。
func mapError(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return bizerrors.New(bizerrors.CodeDBNotFound, notFoundMsg).WithError(err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return bizerrors.New(bizerrors.CodeDBDuplicate, "record already exists").WithError(err)
		case "23503": // foreign_key_violation
			return bizerrors.New(bizerrors.CodeDBConstraint, "referenced record does not exist").WithError(err)
		case "23514": // check_violation
			return bizerrors.New(bizerrors.CodeDBConstraint, "data constraint conflict").WithError(err)
		}
	}
	return bizerrors.New(bizerrors.CodeDBError, "database error").WithError(err)
}

// isNoRows 判断是否为"无记录"。
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ph 返回第 i 个占位符（PostgreSQL 的 $i 风格）。
func ph(i int) string { return "$" + strconv.Itoa(i) }

// like 构造模糊匹配参数。
func like(s string) string { return "%" + s + "%" }

// queryBuilder 动态拼装带参数的 SQL 片段。
type queryBuilder struct {
	conds []string
	args  []interface{}
}

func (b *queryBuilder) add(cond string, args ...interface{}) {
	b.args = append(b.args, args...)
	b.conds = append(b.conds, cond)
}

// arg 追加参数并返回其占位符。
func (b *queryBuilder) arg(v interface{}) string {
	b.args = append(b.args, v)
	return ph(len(b.args))
}

// where 返回 " WHERE c1 AND c2"（无条件时为空串）。
func (b *queryBuilder) where() string {
	if len(b.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.conds, " AND ")
}

// orderLimit 追加排序与分页片段。
func (b *queryBuilder) orderLimit(order string, offset, limit int) string {
	sql := ""
	if order != "" {
		sql += " ORDER BY " + order
	}
	if limit > 0 {
		b.args = append(b.args, limit, offset)
		sql += " LIMIT " + ph(len(b.args)-1) + " OFFSET " + ph(len(b.args))
	}
	return sql
}

// orEmpty 把 nil 切片替换为空切片（避免 NOT NULL 数组 / jsonb 列写入 NULL）。
func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// orEmptyMap 把 nil map 替换为空 map（避免 NOT NULL jsonb 列写入 NULL）。
func orEmptyMap(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}

// reservedColumns 在 SQL 中必须加双引号才能作为列名使用的保留字。
// 例：app_accounts / provision_history 的 "user"、syncers 的 "table"。
var reservedColumns = map[string]bool{
	"user": true, "table": true, "group": true, "order": true,
	"select": true, "from": true, "where": true, "default": true, "check": true,
}

// columnExpr 返回可用于 SQL 片段的列表达式（保留字加引号，其余原样）。
func columnExpr(field string) string {
	if reservedColumns[field] {
		return `"` + field + `"`
	}
	return field
}

// intervalString 把时长转成 PostgreSQL interval 字面量（如 "90 seconds"）。
func intervalString(d time.Duration) string {
	if d <= 0 {
		d = time.Second
	}
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + " seconds"
}

// leaseInterval 租约时长字面量
func leaseInterval(d time.Duration) string { return intervalString(d) }

// nullableJSONB 空载荷落 {}::jsonb（避免 NOT NULL 列收到 NULL）
func nullableJSONB(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

// isUniqueViolation 是否为唯一约束冲突（单飞键并发入队时会遇到）
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// existingNames 返回 names 中确实存在于本组织（institutionID）的标识名集合。
//
// 用途：应用授权名单的逐项校验（不存在的主体一律拒绝，不做静默忽略——否则会出现
// "界面上加了、运行时不生效"的假配置）。表名只允许 groups / departments 这两张
// "标识名在组织内唯一"的目录表，其余调用一律 panic（开发期即暴露，避免拼接未知标识符）。
func existingNames(ctx context.Context, db *dbclient.Client, table string, institutionID int64, names []string) (map[string]bool, error) {
	if table != "groups" && table != "departments" {
		panic("existingNames: 不支持的表 " + table)
	}
	out := map[string]bool{}
	if len(names) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT name FROM `+table+` WHERE institution_id = $1 AND name = ANY($2)`, institutionID, names)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, mapError(err, "")
		}
		out[name] = true
	}
	return out, mapError(rows.Err(), "")
}
