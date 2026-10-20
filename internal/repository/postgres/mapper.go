package postgres

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"polaris/pkg/dbclient"
)

// 本文件提供基于 `db` tag 的通用行映射，替代"每个实体手写三份列清单（SELECT/INSERT/UPDATE）"。
// 动机：应用实体已逾 40 列，手写清单极易漏列（历史上已两次因漏列导致写入失败）。
//
// 约定：
//   - 结构体字段必须以 `db:"<列名>"` 标注；`db:"-"` 表示非持久化字段（如由关联表装配的切片）；
//   - nil 切片与 nil map 在写入前统一归一化为空集合，避免 NOT NULL 列收到 NULL（23502）；
//   - 时间字段使用 *time.Time（可空）或 time.Time（非空）；jsonb 列直接使用 Go 结构体/切片/map，
//     由 pgx 的 JSONB 编解码器负责序列化。

// fieldPlan 描述一个实体的持久化字段布局
type fieldPlan struct {
	columns []string
	indexes []int
}

var planCache = map[reflect.Type]*fieldPlan{}

// planFor 解析实体字段布局（按结构体声明顺序）。
func planFor(v interface{}) (*fieldPlan, error) {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("仅支持结构体实体，实际为 %s", t.Kind())
	}
	if cached, ok := planCache[t]; ok {
		return cached, nil
	}

	plan := &fieldPlan{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		plan.columns = append(plan.columns, tag)
		plan.indexes = append(plan.indexes, i)
	}
	if len(plan.columns) == 0 {
		return nil, fmt.Errorf("实体 %s 缺少 db tag", t.Name())
	}
	planCache[t] = plan
	return plan, nil
}

// normalizeValue 把 nil 集合归一化为空集合（避免 NOT NULL 列写入 NULL）。
func normalizeValue(v reflect.Value) interface{} {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0).Interface()
		}
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMap(v.Type()).Interface()
		}
	}
	return v.Interface()
}

// fieldValue 取出字段值（自动解引用指针）。
func fieldValue(v reflect.Value) interface{} {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return normalizeValue(v.Elem())
	}
	return normalizeValue(v)
}

// rowArgs 返回按列顺序排列的参数值。
func rowArgs(v interface{}) ([]interface{}, error) {
	plan, err := planFor(v)
	if err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	args := make([]interface{}, 0, len(plan.indexes))
	for _, idx := range plan.indexes {
		args = append(args, fieldValue(rv.Field(idx)))
	}
	return args, nil
}

// scanTargets 返回按列顺序排列的扫描目标地址。
func scanTargets(v interface{}) ([]interface{}, error) {
	plan, err := planFor(v)
	if err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	targets := make([]interface{}, 0, len(plan.indexes))
	for _, idx := range plan.indexes {
		targets = append(targets, rv.Field(idx).Addr().Interface())
	}
	return targets, nil
}

// columnList 返回逗号分隔的列名。
func columnList(v interface{}) (string, error) {
	plan, err := planFor(v)
	if err != nil {
		return "", err
	}
	return strings.Join(plan.columns, ", "), nil
}

// insertColumns 返回参与 INSERT 的列（排除自增主键与时间戳列）。
//
// created_at / updated_at 一律由数据库 DEFAULT NOW() 生成，绝不由 Go 写入：
// Go 的零值时间会被当成显式值落库，写成 0001-01-01（历史缺陷，见 AGENTS.md 教训表）。
func insertColumns(v interface{}) ([]string, []interface{}, error) {
	plan, err := planFor(v)
	if err != nil {
		return nil, nil, err
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	columns := make([]string, 0, len(plan.columns))
	args := make([]interface{}, 0, len(plan.columns))
	for i, col := range plan.columns {
		if insertSkippedColumns[col] {
			continue
		}
		columns = append(columns, col)
		args = append(args, fieldValue(rv.Field(plan.indexes[i])))
	}
	return columns, args, nil
}

// insertSkippedColumns INSERT 阶段由数据库负责生成的列。
var insertSkippedColumns = map[string]bool{"id": true, "created_at": true, "updated_at": true}

// insertReturningID 插入实体并回填自增主键与数据库生成的时间戳。
//
// created_at / updated_at 由数据库 DEFAULT NOW() 生成后随 RETURNING 回填到实体，
// 保证"刚创建的对象"在响应里就带有正确时间（不再出现零值时间）。
type rowQuerier interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

func insertReturningID(ctx context.Context, db rowQuerier, table string, v interface{}) (int64, error) {
	cols, args, err := insertColumns(v)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("实体 %s 没有可插入的列", table)
	}
	plan, err := planFor(v)
	if err != nil {
		return 0, err
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	placeholders := make([]string, 0, len(args))
	for i := range args {
		placeholders = append(placeholders, ph(i+1))
	}
	returning := []string{"id"}
	var createdAt, updatedAt time.Time
	withCreated := hasColumn(plan, "created_at")
	withUpdated := hasColumn(plan, "updated_at")
	if withCreated {
		returning = append(returning, "created_at")
	}
	if withUpdated {
		returning = append(returning, "updated_at")
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		table, strings.Join(cols, ", "), strings.Join(placeholders, ", "), strings.Join(returning, ", "))

	var id int64
	dest := make([]interface{}, 0, len(returning))
	dest = append(dest, &id)
	if withCreated {
		dest = append(dest, &createdAt)
	}
	if withUpdated {
		dest = append(dest, &updatedAt)
	}
	if err := db.QueryRow(ctx, sql, args...).Scan(dest...); err != nil {
		return 0, mapError(err, notFoundMessage(table))
	}
	if withCreated {
		assignTimeField(rv, plan, "created_at", createdAt)
	}
	if withUpdated {
		assignTimeField(rv, plan, "updated_at", updatedAt)
	}
	return id, nil
}

// assignTimeField 把数据库生成的时间戳写回实体字段（兼容 time.Time 与 *time.Time）。
func assignTimeField(rv reflect.Value, plan *fieldPlan, column string, at time.Time) {
	timeType := reflect.TypeOf(time.Time{})
	for i, col := range plan.columns {
		if col != column {
			continue
		}
		field := rv.Field(plan.indexes[i])
		switch {
		case field.Kind() == reflect.Struct && field.Type() == timeType:
			field.Set(reflect.ValueOf(at))
		case field.Kind() == reflect.Pointer && field.Type().Elem() == timeType:
			value := at
			field.Set(reflect.ValueOf(&value))
		}
		return
	}
}

// selectOne 按条件查询单行并扫描进实体。
func selectOne(ctx context.Context, db *dbclient.Client, table string, v interface{}, where string, args ...interface{}) error {
	cols, err := columnList(v)
	if err != nil {
		return err
	}
	targets, err := scanTargets(v)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("SELECT %s FROM %s %s", cols, table, where)
	if err := db.QueryRow(ctx, sql, args...).Scan(targets...); err != nil {
		return mapError(err, notFoundMessage(table))
	}
	return nil
}

// queryEntities 按条件查询多行并扫描进实体切片（泛型版，返回 *T 列表）。
func queryEntities[T any](ctx context.Context, db *dbclient.Client, table, where string, args []interface{}, order string, offset, limit int) ([]*T, error) {
	prototype := new(T)
	cols, err := columnList(prototype)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf("SELECT %s FROM %s %s", cols, table, where)
	if order != "" {
		sql += " ORDER BY " + order
	}
	if limit > 0 {
		args = append(args, limit, offset)
		sql += fmt.Sprintf(" LIMIT %s OFFSET %s", ph(len(args)-1), ph(len(args)))
	}

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()

	var result []*T
	for rows.Next() {
		entity := new(T)
		targets, err := scanTargets(entity)
		if err != nil {
			return nil, err
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, entity)
	}
	return result, mapError(rows.Err(), "")
}

// countEntities 统计行数。
func countEntities(ctx context.Context, db *dbclient.Client, table, where string, args ...interface{}) (int64, error) {
	var n int64
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s %s", table, where), args...).Scan(&n); err != nil {
		return 0, mapError(err, "")
	}
	return n, nil
}

// updateEntity 按主键更新实体；immutable 中的列不参与更新。
// updated_at 由本方法统一维护（若实体含该列）。
func updateEntity(ctx context.Context, db *dbclient.Client, table string, v interface{}, immutable ...string) error {
	plan, err := planFor(v)
	if err != nil {
		return err
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	// updated_at 由本方法统一写入 NOW()，需从字段赋值中排除（否则同一列被赋值两次）
	// 租户列（institution_id）是归属身份，绝不允许被 UPDATE 改写
	skip := map[string]bool{"id": true, "created_at": true, "updated_at": true, "institution_id": true}
	for _, name := range immutable {
		skip[name] = true
	}

	assignments := make([]string, 0, len(plan.columns))
	args := make([]interface{}, 0, len(plan.columns))
	for i, col := range plan.columns {
		if skip[col] {
			continue
		}
		args = append(args, fieldValue(rv.Field(plan.indexes[i])))
		assignments = append(assignments, fmt.Sprintf("%s = %s", col, ph(len(args))))
	}
	if hasColumn(plan, "updated_at") {
		assignments = append(assignments, "updated_at = NOW()")
	}

	id, ok := idOf(rv, plan)
	if !ok {
		return fmt.Errorf("实体缺少可用的 id 字段用于更新")
	}
	args = append(args, id)
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = %s", table, strings.Join(assignments, ", "), ph(len(args)))
	return mapError(db.Exec(ctx, sql, args...), notFoundMessage(table))
}

func hasColumn(plan *fieldPlan, name string) bool {
	for _, col := range plan.columns {
		if col == name {
			return true
		}
	}
	return false
}

func idOf(rv reflect.Value, plan *fieldPlan) (int64, bool) {
	for i, col := range plan.columns {
		if col != "id" {
			continue
		}
		field := rv.Field(plan.indexes[i])
		if field.Kind() == reflect.Int64 {
			return field.Int(), true
		}
	}
	return 0, false
}

// notFoundMessage 依据表名给出统一的"未找到"文案。
func notFoundMessage(table string) string {
	switch table {
	case "applications":
		return "application not found"
	case "providers":
		return "provider not found"
	case "users":
		return "user not found"
	case "institutions":
		return "institution not found"
	case "roles":
		return "role not found"
	case "groups":
		return "group not found"
	case "certs":
		return "certificate not found"
	case "tokens":
		return "token not found"
	default:
		return "record not found"
	}
}

var (
	_ = pgx.ErrNoRows
	_ = time.Now
)
