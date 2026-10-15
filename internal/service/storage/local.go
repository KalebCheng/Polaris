package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStore 本地磁盘存储（开发或单机部署；生产建议对象存储）
type LocalStore struct{ cfg Config }

func newLocalStore(cfg Config) (*LocalStore, error) {
	if err := os.MkdirAll(cfg.RootPath, 0o755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}
	return &LocalStore{cfg: cfg}, nil
}

// Kind 后端类型
func (s *LocalStore) Kind() string { return "local" }

// Put 写入文件并返回公开访问 URL
func (s *LocalStore) Put(_ context.Context, obj Object) (string, error) {
	key := withPrefix(s.cfg.PathPrefix, obj.Key)
	full := filepath.Join(s.cfg.RootPath, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	file, err := os.Create(full)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, obj.Body); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return s.PublicURL(key), nil
}

// PublicURL 本地文件的公开访问地址
func (s *LocalStore) PublicURL(key string) string {
	base := strings.TrimSuffix(s.cfg.PublicBase, "/")
	if s.cfg.Domain != "" {
		base = strings.TrimSuffix(s.cfg.Domain, "/")
	}
	return base + "/static/storage/" + key
}

// Delete 删除文件
func (s *LocalStore) Delete(_ context.Context, key string) error {
	full := filepath.Join(s.cfg.RootPath, filepath.FromSlash(withPrefix(s.cfg.PathPrefix, key)))
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Probe 连通性自检
func (s *LocalStore) Probe(_ context.Context) error {
	probe := fmt.Sprintf(".probe-%d", time.Now().UnixNano())
	path := filepath.Join(s.cfg.RootPath, probe)
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("写入探测文件失败: %w", err)
	}
	defer func() { _ = os.Remove(path) }()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("读取探测文件失败: %w", err)
	}
	return nil
}
