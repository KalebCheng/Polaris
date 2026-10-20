// Package storage 实现"存储类提供商"：对象存储（S3 / MinIO 兼容）与本地磁盘。
//
// 使用场景：用户头像等由平台托管的文件。业务层只依赖 Store 接口，
// 新增后端只需实现接口并在 Factory 中注册。
package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Config 存储配置（来自 providers 表 category=Storage）
type Config struct {
	Type       string // local | s3 | minio
	Endpoint   string
	Region     string
	Bucket     string
	AccessKey  string
	SecretKey  string
	PathPrefix string
	Domain     string // 自定义访问域名（CDN / 反代），空 = 使用对象存储直链
	RootPath   string // 本地模式根目录
	PublicBase string // 本地模式的公开访问前缀
	PathStyle  bool
}

// Object 待写入的对象
type Object struct {
	Key         string
	Body        io.Reader
	ContentType string
	Size        int64
}

// Store 存储后端
type Store interface {
	// Kind 后端类型
	Kind() string
	// Put 写入对象并返回可访问 URL
	Put(ctx context.Context, obj Object) (string, error)
	// Delete 删除对象
	Delete(ctx context.Context, key string) error
	// Probe 连通性自检（写入并删除探测对象）
	Probe(ctx context.Context) error
}

// New 依据配置构造存储后端
func New(cfg Config) (Store, error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch kind {
	case "local", "":
		if cfg.RootPath == "" {
			return nil, fmt.Errorf("本地存储缺少根目录")
		}
		return newLocalStore(cfg)
	case "s3", "minio", "oss", "cos":
		if cfg.Bucket == "" {
			return nil, fmt.Errorf("对象存储缺少 Bucket")
		}
		return newS3Store(cfg)
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", kind)
	}
}

// sanitizeKey 规范化对象键：去掉前导斜杠与 .. 片段，防止路径穿越。
func sanitizeKey(key string) string {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	parts := make([]string, 0, 8)
	for _, seg := range strings.Split(key, "/") {
		switch seg {
		case "", ".", "..":
			continue
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, "/")
}

// withPrefix 拼接配置的路径前缀
func withPrefix(prefix, key string) string {
	clean := sanitizeKey(key)
	if prefix == "" {
		return clean
	}
	return strings.Trim(sanitizeKey(prefix), "/") + "/" + clean
}
