package syncer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	// MySQL 驱动：此前 MySQL 分支 sql.Open("mysql") 必然失败（驱动未注册）
	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
)

// DatabaseConnector 关系数据库连接器（PostgreSQL / MySQL）。
// 通过 user_mapping（本地字段 → 源列名）映射字段；table 为源表名。
type DatabaseConnector struct{}

// Type 连接器类型
func (c *DatabaseConnector) Type() string { return "database" }

func databaseType(s *repository.Syncer) string {
	t := strings.ToLower(s.DatabaseType)
	if t == "" {
		t = strings.ToLower(s.SubType)
	}
	if t == "" {
		t = "postgres"
	}
	return t
}

// FetchUsers 从数据库拉取用户
func (c *DatabaseConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	switch databaseType(s) {
	case "mysql", "mariadb":
		return c.fetchMySQL(ctx, s)
	default:
		return c.fetchPostgres(ctx, s)
	}
}

// FetchGroups 数据库连接器不提供用户组
func (c *DatabaseConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	return nil, nil
}

func mapRows(columns []string, rows [][]string, mapping map[string]string) []ExternalUser {
	index := map[string]int{}
	for i, col := range columns {
		index[strings.ToLower(col)] = i
	}
	pick := func(row []string, candidates []string) string {
		for _, cand := range candidates {
			if i, ok := index[strings.ToLower(cand)]; ok && i < len(row) && row[i] != "" {
				return row[i]
			}
		}
		return ""
	}
	users := make([]ExternalUser, 0, len(rows))
	for _, row := range rows {
		lookup := func(field string) string {
			if i, ok := index[strings.ToLower(field)]; ok && i < len(row) {
				return row[i]
			}
			return ""
		}
		// 先按字段映射（支持"备选源字段"与"固定值"），未配置或取空则回退到内置候选列名
		value := func(localKey string, fallback []string) string {
			if v, ok := ResolveMapping(mapping, localKey, lookup); ok {
				return v
			}
			return pick(row, fallback)
		}
		u := ExternalUser{
			ID:       value("id", []string{"id", "username", "user_id", "userid", "uid", "name"}),
			Username: value("name", []string{"username", "user_name", "name", "uid", "account"}),
			Name:     value("displayName", []string{"display_name", "name", "nickname", "full_name"}),
			Email:    value("email", []string{"email", "mail"}),
			Phone:    value("phone", []string{"phone", "mobile", "telephone"}),
			Avatar:   value("avatar", []string{"avatar", "avatar_url", "picture"}),
			Title:    value("title", []string{"title", "position", "job_title"}),
			Location: value("location", []string{"location", "city", "address"}),
		}
		if u.ID == "" {
			u.ID = u.Username
		}
		if u.Username == "" {
			u.Username = u.ID
		}
		if u.Name == "" {
			u.Name = u.Username
		}
		users = append(users, u)
	}
	return users
}

func (c *DatabaseConnector) fetchPostgres(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	sslmode := s.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		s.Host, s.Port, s.User, s.Password, s.Database, sslmode)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, fmt.Sprintf(`SELECT * FROM %s`, quoteIdent(s.Table)))
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	cols := rows.FieldDescriptions()
	columns := make([]string, 0, len(cols))
	for _, fd := range cols {
		columns = append(columns, string(fd.Name))
	}
	var table [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(values))
		for i, v := range values {
			if v != nil {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		table = append(table, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mapRows(columns, table, s.UserMapping), nil
}

func (c *DatabaseConnector) fetchMySQL(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true", s.User, s.Password, s.Host, s.Port, s.Database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", quoteIdent(s.Table)))
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var table [][]string
	for rows.Next() {
		raw := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]string, len(columns))
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else if v != nil {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		table = append(table, row)
	}
	return mapRows(columns, table, s.UserMapping), nil
}

// quoteIdent 简单标识符引用（仅允许字母数字下划线，避免注入）。
func quoteIdent(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
