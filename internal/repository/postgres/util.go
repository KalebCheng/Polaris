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
	"polaris/internal/repository"
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

// applyQueryFilters 把 ListQuery.Filters 追加为等值条件（AND 语义）。
// 列名只来自服务层构造的固定字段，不接受外部原文；调用方仍需保证列存在。
func applyQueryFilters(b *queryBuilder, filters []repository.ColumnFilter) {
	for _, f := range filters {
		if f.Field == "" {
			continue
		}
		b.add(`t.` + columnExpr(f.Field) + `=` + b.arg(f.Value))
	}
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
// 例：application_accounts / provision_history 的 "user"、syncers 的 "table"。
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

// existingIDs returns tenant-local primary keys from an allowlisted directory table.
func existingIDs(ctx context.Context, db *dbclient.Client, table string, institutionID int64, ids []int64) (map[int64]bool, error) {
	if table != "job_titles" && table != "departments" {
		panic("existingIDs: unsupported table " + table)
	}
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT id FROM `+table+` WHERE institution_id = $1 AND id = ANY($2)`, institutionID, ids)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = true
	}
	return out, mapError(rows.Err(), "")
}

// idFilterValue 过滤字段以 _id 结尾时按 BIGINT 传参。
// 动机：ListQuery.Value 是字符串（历史约定），而主键列是 bigint——直接传字符串会触发
// PostgreSQL 的 "bigint = text" 类型冲突（§3.6 的同一类坑）。这里只做类型转换，不做语义兜底。
func idFilterValue(field, value string) interface{} {
	if strings.HasSuffix(field, "_id") {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return n
		}
	}
	return value
}
