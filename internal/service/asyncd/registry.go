package asyncd

import (
	"context"
	"fmt"
	"sort"
	"time"

	"polaris/internal/repository"
)

// Handler 任务处理器：payload 为已解码的 JSON 对象；返回 error 表示本次执行失败（按策略重试）。
type Handler func(ctx context.Context, run *repository.AsyncdJobRun, payload map[string]any) error

// HandlerOption 处理器选项
type HandlerOption func(*handlerEntry)

type handlerEntry struct {
	handler  Handler
	timeout  time.Duration
	maxRetry int
}

// WithTimeout 覆盖该类型任务的执行超时（默认取任务的 deadline，未设置则不限制）
func WithTimeout(d time.Duration) HandlerOption {
	return func(e *handlerEntry) { e.timeout = d }
}

// HandlerMaxAttempts 覆盖该类型任务的最大尝试次数
func HandlerMaxAttempts(n int) HandlerOption {
	return func(e *handlerEntry) {
		if n > 0 {
			e.maxRetry = n
		}
	}
}

// Registry 处理器注册表：任务类型 → 处理器（新增任务类型只在这里登记一处）。
type Registry struct {
	entries map[string]handlerEntry
}

// NewRegistry 创建注册表
func NewRegistry() *Registry { return &Registry{entries: map[string]handlerEntry{}} }

// Register 注册处理器；重复注册或空类型会 panic（启动期配置错误，越早暴露越好）
func (r *Registry) Register(taskType string, handler Handler, opts ...HandlerOption) {
	if taskType == "" || handler == nil {
		panic("task: 处理器类型与实现均不能为空")
	}
	if _, exists := r.entries[taskType]; exists {
		panic(fmt.Sprintf("task: 任务类型 %s 重复注册", taskType))
	}
	entry := handlerEntry{handler: handler}
	for _, opt := range opts {
		opt(&entry)
	}
	r.entries[taskType] = entry
}

// Lookup 查找处理器
func (r *Registry) Lookup(taskType string) (handlerEntry, bool) {
	entry, ok := r.entries[taskType]
	return entry, ok
}

// Types 已注册的任务类型（测试与管理界面用）
func (r *Registry) Types() []string {
	out := make([]string, 0, len(r.entries))
	for taskType := range r.entries {
		out = append(out, taskType)
	}
	sort.Strings(out)
	return out
}
