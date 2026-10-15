// Package migrator 提供轻量级数据库迁移执行器。
//
// 设计（与 SkyPivot 保持一致）：
//   - 文件命名兼容 golang-migrate（NNNN_name.up.sql / NNNN_name.down.sql）；
//   - 迁移记录写入 schema_migrations(version, name, applied_at) 表；
//   - 每个迁移在独立事务中执行：失败即回滚该迁移并停止（不记录版本）；
//   - 使用 PostgreSQL advisory lock 防止多实例并发迁移。
package migrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration 一条迁移
type Migration struct {
	Version int64
	Name    string
	Up      string
	Down    string
}

var (
	upPattern     = regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)
	downPattern   = regexp.MustCompile(`^(\d+)_(.+)\.down\.sql$`)
	legacyPattern = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)
)

// advisoryLockKey advisory lock 常量键（任意固定值，避免与其他锁冲突）
const advisoryLockKey = 0x53494D47 // "SIMG"

const createSchemaTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version  BIGINT PRIMARY KEY,
	name     TEXT NOT NULL DEFAULT '',
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// LoadDir 扫描迁移目录并按版本排序；忽略不匹配的文件。
func LoadDir(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录失败 %s: %w", dir, err)
	}

	byVersion := make(map[int64]*Migration)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()

		var (
			version int64
			migName string
			hasUp   bool
			hasDown bool
		)
		switch {
		case upPattern.MatchString(name):
			m := upPattern.FindStringSubmatch(name)
			version, migName, hasUp = parseInt(m[1]), m[2], true
		case downPattern.MatchString(name):
			m := downPattern.FindStringSubmatch(name)
			version, migName, hasDown = parseInt(m[1]), m[2], true
		case legacyPattern.MatchString(name):
			m := legacyPattern.FindStringSubmatch(name)
			version, migName, hasUp = parseInt(m[1]), m[2], true
		default:
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("读取迁移文件失败 %s: %w", name, err)
		}

		m, exists := byVersion[version]
		if !exists {
			m = &Migration{Version: version, Name: migName}
			byVersion[version] = m
		}
		if hasUp {
			m.Up = string(data)
		}
		if hasDown {
			m.Down = string(data)
		}
	}

	versions := make([]int64, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })

	result := make([]Migration, 0, len(versions))
	for _, v := range versions {
		result = append(result, *byVersion[v])
	}
	return result, nil
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// Up 应用所有未执行的迁移，返回本次应用的版本列表。
func Up(ctx context.Context, pool *pgxpool.Pool, dir string) ([]int64, error) {
	migrations, err := LoadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(migrations) == 0 {
		return nil, nil
	}

	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := pool.Acquire(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("获取迁移连接失败: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_lock(%d)", advisoryLockKey)); err != nil {
		return nil, fmt.Errorf("获取迁移锁失败: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), fmt.Sprintf("SELECT pg_advisory_unlock(%d)", advisoryLockKey))
	}()

	if _, err := conn.Exec(ctx, createSchemaTableSQL); err != nil {
		return nil, fmt.Errorf("创建迁移记录表失败: %w", err)
	}

	applied := make(map[int64]bool)
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("查询已应用迁移失败: %w", err)
	}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return nil, fmt.Errorf("扫描已应用迁移失败: %w", err)
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历已应用迁移失败: %w", err)
	}

	var appliedList []int64
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if m.Up == "" {
			return appliedList, fmt.Errorf("迁移 %d_%s 缺少 up 文件，终止", m.Version, m.Name)
		}

		tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return appliedList, fmt.Errorf("开启迁移 %d 事务失败: %w", m.Version, err)
		}
		if _, err := tx.Exec(ctx, m.Up); err != nil {
			_ = tx.Rollback(ctx)
			return appliedList, fmt.Errorf("应用迁移 %d_%s 失败: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name); err != nil {
			_ = tx.Rollback(ctx)
			return appliedList, fmt.Errorf("记录迁移 %d_%s 失败: %w", m.Version, m.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return appliedList, fmt.Errorf("提交迁移 %d_%s 失败: %w", m.Version, m.Name, err)
		}
		appliedList = append(appliedList, m.Version)
	}
	return appliedList, nil
}

// Down 回滚最近 steps 步迁移（按版本倒序）。
func Down(ctx context.Context, pool *pgxpool.Pool, dir string, steps int) ([]int64, error) {
	if steps <= 0 {
		return nil, nil
	}
	migrations, err := LoadDir(dir)
	if err != nil {
		return nil, err
	}
	byVersion := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byVersion[m.Version] = m
	}

	lockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, err := pool.Acquire(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("获取迁移连接失败: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_lock(%d)", advisoryLockKey)); err != nil {
		return nil, fmt.Errorf("获取迁移锁失败: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), fmt.Sprintf("SELECT pg_advisory_unlock(%d)", advisoryLockKey))
	}()

	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT $1`, steps)
	if err != nil {
		return nil, fmt.Errorf("查询待回滚迁移失败: %w", err)
	}
	var rollbackVersions []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return nil, fmt.Errorf("扫描待回滚迁移失败: %w", err)
		}
		rollbackVersions = append(rollbackVersions, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历待回滚迁移失败: %w", err)
	}

	var rolledBack []int64
	for _, v := range rollbackVersions {
		m, ok := byVersion[v]
		if !ok {
			return rolledBack, fmt.Errorf("迁移 %d 已在数据库记录但目录中不存在，终止", v)
		}
		if m.Down == "" {
			return rolledBack, fmt.Errorf("迁移 %d_%s 缺少 down 文件，无法回滚（终止）", m.Version, m.Name)
		}
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return rolledBack, fmt.Errorf("开启回滚迁移 %d 事务失败: %w", v, err)
		}
		if _, err := tx.Exec(ctx, m.Down); err != nil {
			_ = tx.Rollback(ctx)
			return rolledBack, fmt.Errorf("回滚迁移 %d_%s 失败: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, v); err != nil {
			_ = tx.Rollback(ctx)
			return rolledBack, fmt.Errorf("移除迁移 %d 记录失败: %w", m.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return rolledBack, fmt.Errorf("提交回滚迁移 %d 失败: %w", m.Version, err)
		}
		rolledBack = append(rolledBack, v)
	}
	return rolledBack, nil
}
