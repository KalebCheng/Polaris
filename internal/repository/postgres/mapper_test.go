package postgres

import (
	"strings"
	"testing"
	"time"
)

type mapperSample struct {
	ID          int64             `db:"id"`
	Institution string            `db:"institution_id"`
	Name        string            `db:"name"`
	Tags        []string          `db:"tags"`
	Options     map[string]string `db:"options"`
	Limit       *int              `db:"limit_value"`
	Comment     string            `db:"-"`
	CreatedAt   time.Time         `db:"created_at"`
	UpdatedAt   time.Time         `db:"updated_at"`
}

func TestPlanForSkipsUntaggedColumns(t *testing.T) {
	plan, err := planFor(&mapperSample{})
	if err != nil {
		t.Fatalf("planFor 失败: %v", err)
	}
	joined := strings.Join(plan.columns, ",")
	if strings.Contains(joined, "comment") {
		t.Fatalf("db:\"-\" 字段不应进入列清单: %s", joined)
	}
	if len(plan.columns) != 8 {
		t.Fatalf("列数 = %d, 期望 8 (%s)", len(plan.columns), joined)
	}
}

// 回归：INSERT 不得包含自增主键，否则会以 id=0 写入并与序列冲突（曾导致 409 duplicate pkey）。
func TestInsertColumnsExcludesID(t *testing.T) {
	cols, args, err := insertColumns(&mapperSample{Name: "a", Tags: []string{"x"}})
	if err != nil {
		t.Fatalf("insertColumns 失败: %v", err)
	}
	if len(cols) != len(args) {
		t.Fatalf("列与参数数量不一致: %d vs %d", len(cols), len(args))
	}
	for _, col := range cols {
		if col == "id" {
			t.Fatal("INSERT 不应包含自增主键 id")
		}
	}
	if len(cols) != 5 {
		t.Fatalf("INSERT 列数 = %d, 期望 5", len(cols))
	}
}

// 回归：INSERT 不得包含 created_at / updated_at。
// 事故：Go 零值时间被当成显式值写入，行落成 0001-01-01（应用/提供商实体均中招）。
func TestInsertColumnsExcludesTimestamps(t *testing.T) {
	cols, _, err := insertColumns(&mapperSample{})
	if err != nil {
		t.Fatalf("insertColumns 失败: %v", err)
	}
	for _, col := range cols {
		if col == "created_at" || col == "updated_at" {
			t.Fatalf("INSERT 不应包含时间戳列 %s（应由数据库 DEFAULT NOW() 生成）", col)
		}
	}
}

// 回归：nil 切片/映射必须归一化为空集合，避免 NOT NULL 列写入 NULL（23502）。
func TestRowArgsNormalizesNilCollections(t *testing.T) {
	args, err := rowArgs(&mapperSample{})
	if err != nil {
		t.Fatalf("rowArgs 失败: %v", err)
	}
	tags, ok := args[3].([]string)
	if !ok || tags == nil {
		t.Fatalf("nil 切片应归一化为非 nil 空切片，实际 %#v", args[3])
	}
	options, ok := args[4].(map[string]string)
	if !ok || options == nil {
		t.Fatalf("nil map 应归一化为非 nil 空 map，实际 %#v", args[4])
	}
	// 指针字段保持 nil 语义
	if args[5] != nil {
		t.Fatalf("nil 指针应保持 nil，实际 %#v", args[5])
	}
}

func TestUpdateSkipsImmutableAndTimestampColumns(t *testing.T) {
	// updateEntity 的列筛选逻辑：id / created_at / updated_at 与显式不可变列均不参与赋值
	plan, err := planFor(&mapperSample{})
	if err != nil {
		t.Fatalf("planFor 失败: %v", err)
	}
	skip := map[string]bool{"id": true, "created_at": true, "updated_at": true, "institution_id": true, "name": true}
	var assigned []string
	for _, col := range plan.columns {
		if !skip[col] {
			assigned = append(assigned, col)
		}
	}
	joined := strings.Join(assigned, ",")
	for _, forbidden := range []string{"id", "created_at", "updated_at", "institution_id", "name"} {
		for _, col := range strings.Split(joined, ",") {
			if col == forbidden {
				t.Fatalf("列 %s 不应出现在 UPDATE 赋值中", forbidden)
			}
		}
	}
	if len(assigned) != 3 {
		t.Fatalf("可更新列 = %v, 期望 3 列", assigned)
	}
}
