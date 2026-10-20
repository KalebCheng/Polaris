package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// 回归：nil 切片/映射直接写入 NOT NULL 队列会触发 23502；仓库层必须统一兜底为空集合。
func TestOrEmpty(t *testing.T) {
	var s []string
	if got := orEmpty(s); got == nil || len(got) != 0 {
		t.Fatal("nil 切片应转换为空切片")
	}
	in := []string{"a"}
	if got := orEmpty(in); len(got) != 1 || got[0] != "a" {
		t.Fatal("非空切片应原样返回")
	}
	var m map[string]string
	if got := orEmptyMap(m); got == nil || len(got) != 0 {
		t.Fatal("nil map 应转换为空 map")
	}
}

func TestMapError(t *testing.T) {
	if err := mapError(nil, ""); err != nil {
		t.Fatalf("nil 应返回 nil，实际 %v", err)
	}
	if err := mapError(pgx.ErrNoRows, "not found"); !bizerrors.Is(err, bizerrors.CodeDBNotFound) {
		t.Fatalf("ErrNoRows 应映射为 DB_NOT_FOUND，实际 %v", err)
	}
	unique := &pgconn.PgError{Code: "23505"}
	if err := mapError(unique, ""); !bizerrors.Is(err, bizerrors.CodeDBDuplicate) {
		t.Fatalf("23505 应映射为 DB_DUPLICATE，实际 %v", err)
	}
	fk := &pgconn.PgError{Code: "23503"}
	if err := mapError(fk, ""); !bizerrors.Is(err, bizerrors.CodeDBConstraint) {
		t.Fatalf("23503 应映射为 DB_CONSTRAINT，实际 %v", err)
	}
	// 未知错误一律兜底为 DB_ERROR，且不把底层细节作为 message 下发
	if err := mapError(errors.New("boom"), ""); !bizerrors.Is(err, bizerrors.CodeDBError) {
		t.Fatalf("未知错误应映射为 DB_ERROR，实际 %v", err)
	}
}

func TestQueryBuilder(t *testing.T) {
	b := &queryBuilder{}
	b.add(`institution_id=` + b.arg("built-in"))
	p := b.arg(like("admin"))
	b.add(`(name ILIKE ` + p + ` OR email ILIKE ` + p + `)`)
	where := b.where()
	want := ` WHERE institution_id=$1 AND (name ILIKE $2 OR email ILIKE $2)`
	if where != want {
		t.Fatalf("where = %q, 期望 %q", where, want)
	}
	if len(b.args) != 2 {
		t.Fatalf("参数个数 = %d, 期望 2", len(b.args))
	}
	if b.args[1] != "%admin%" {
		t.Fatalf("模糊匹配参数 = %v", b.args[1])
	}

	suffix := b.orderLimit("name ASC", 20, 10)
	if suffix != " ORDER BY name ASC LIMIT $3 OFFSET $4" {
		t.Fatalf("orderLimit = %q", suffix)
	}

	empty := &queryBuilder{}
	if empty.where() != "" {
		t.Fatal("无条件时 where 应为空串")
	}
}

func TestDefaultStr(t *testing.T) {
	if defaultStr("", "bcrypt") != "bcrypt" {
		t.Fatal("空值应回退默认值")
	}
	if defaultStr("plain", "bcrypt") != "plain" {
		t.Fatal("非空值应原样返回")
	}
}

// 回归：SQL 保留字列（如 application_accounts."user"）作为过滤字段时必须加引号，
// 否则 `WHERE user=$1` 会解析失败，导致按用户过滤的应用账号/供给历史查询静默返回空。
func TestColumnExprQuotesReserved(t *testing.T) {
	cases := map[string]string{
		"user":          `"user"`,
		"table":         `"table"`,
		"group":         `"group"`,
		"order":         `"order"`,
		"institutionID": "institutionID",
		"application":   "application",
		"parent_id":     "parent_id",
	}
	for field, want := range cases {
		if got := columnExpr(field); got != want {
			t.Fatalf("columnExpr(%q) = %q, 期望 %q", field, got, want)
		}
	}
}

// 回归：应用详情按 application_id、出方向连接按 connector 过滤时，
// 过滤条件必须作为独立的 AND 条件追加，不能被 Keyword 分支吞掉。
func TestApplyQueryFilters(t *testing.T) {
	b := &queryBuilder{}
	applyQueryFilters(b, []repository.ColumnFilter{
		{Field: "application_id", Value: int64(7)},
		{Field: "connector", Value: "dingtalk"},
	})
	if got, want := b.where(), ` WHERE t.application_id=$1 AND t.connector=$2`; got != want {
		t.Fatalf("where = %q, 期望 %q", got, want)
	}
	if len(b.args) != 2 || b.args[0] != int64(7) || b.args[1] != "dingtalk" {
		t.Fatalf("过滤参数错误: %#v", b.args)
	}
}
