// Package dbclient 封装 PostgreSQL 连接池（pgx v5），向上提供事务助手与健康检查。
package dbclient

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Client PostgreSQL 客户端
type Client struct {
	pool *pgxpool.Pool
	cfg  Config
}

// Config PostgreSQL 配置
type Config struct {
	DSN             string
	MaxOpenConns    int32
	MaxIdleConns    int32
	ConnMaxLifetime time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 300 * time.Second,
	}
}

// New 创建 PostgreSQL 客户端
func New(cfg Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("解析 DSN 失败: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxOpenConns
	poolConfig.MinConns = cfg.MaxIdleConns
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("创建连接池失败: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}

	return &Client{pool: pool, cfg: cfg}, nil
}

// Pool 获取连接池
func (c *Client) Pool() *pgxpool.Pool { return c.pool }

// Ping 健康检查
func (c *Client) Ping(ctx context.Context) error { return c.pool.Ping(ctx) }

// Close 关闭连接
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

// Health 健康状态
func (c *Client) Health(ctx context.Context) (bool, error) {
	if err := c.pool.Ping(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Exec 执行 SQL
func (c *Client) Exec(ctx context.Context, sql string, args ...interface{}) error {
	_, err := c.pool.Exec(ctx, sql, args...)
	return err
}

// ExecTag 执行 SQL 并返回命令标签（需要 RowsAffected 判定时使用）
func (c *Client) ExecTag(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return c.pool.Exec(ctx, sql, args...)
}

// WithTx 在事务中执行 fn：fn 返回 nil 则提交，否则回滚。
// 红线：涉及多张表的写操作必须使用事务（见 AGENTS.md §3.1-6）。
func (c *Client) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// QueryRow 查询单行
func (c *Client) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return c.pool.QueryRow(ctx, sql, args...)
}

// Query 查询多行
func (c *Client) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return c.pool.Query(ctx, sql, args...)
}
