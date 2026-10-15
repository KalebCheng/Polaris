package asyncd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"polaris/internal/logger"
	"polaris/internal/repository"
)

// Service 任务与调度的对外入口（入队 / 查询 / 人工干预 / 注册处理器）。
//
// 装配方式遵循 AGENTS.md §3.1-1：由 app 统一构造并注入，不做全局单例。
type Service struct {
	tasks     repository.AsyncdTaskRepository
	schedules repository.AsyncdScheduleRepository
	registry  *Registry
	workerID  string
	clock     func() time.Time
}

// NewService 创建任务服务
func NewService(tasks repository.AsyncdTaskRepository, schedules repository.AsyncdScheduleRepository) *Service {
	return &Service{
		tasks: tasks, schedules: schedules,
		registry: NewRegistry(),
		workerID: instanceID(),
		clock:    time.Now,
	}
}

// instanceID 实例标识：主机名 + 进程号（多实例排查"谁领了这个任务"时一眼可辨）
func instanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "polaris"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// Registry 暴露注册表（供 app 装配时登记处理器）
func (s *Service) Registry() *Registry { return s.registry }

// Enqueue 入队一个任务。
//   - taskType：处理器注册名；
//   - payload：任意可 JSON 序列化的对象（处理器自行按 payloadVersion 兼容）；
//   - uniqueKey：单飞键（同一键在 pending/active 下唯一）；传空表示不去重；
//   - runAt：延迟执行时间（零值 = 立即）。
//
// 返回 existing=true 表示撞上单飞键、复用了已存在的任务（不会重复执行）。
func (s *Service) Enqueue(
	ctx context.Context, taskType string, payload any, uniqueKey string, runAt time.Time, opts ...EnqueueOption,
) (id int64, existing bool, err error) {
	if _, ok := s.registry.Lookup(taskType); !ok {
		// 未注册的任务一律拒绝入队：避免"入了队却没人执行"的静默堆积
		return 0, false, errUnknownTaskType(taskType)
	}
	task := &repository.AsyncdTask{
		Type: taskType, PayloadVersion: 1, MaxAttempts: DefaultMaxAttempts,
		UniqueKey: uniqueKey, NextAttemptAt: runAt,
	}
	for _, opt := range opts {
		opt(task)
	}
	if raw, marshalErr := json.Marshal(payload); marshalErr == nil && payload != nil {
		task.Payload = raw
	}
	return s.tasks.Enqueue(ctx, task)
}

// EnqueueOption 入队选项
type EnqueueOption func(*repository.AsyncdTask)

// WithMaxAttempts 设置最大尝试次数
func WithMaxAttempts(n int) EnqueueOption {
	return func(t *repository.AsyncdTask) {
		if n > 0 {
			t.MaxAttempts = n
		}
	}
}

// WithDeadline 设置任务级截止时间（超期不再领取）
func WithDeadline(at time.Time) EnqueueOption {
	return func(t *repository.AsyncdTask) { t.Deadline = &at }
}

// WithPriority 设置优先级（越大越先被领取）
func WithPriority(p int) EnqueueOption { return func(t *repository.AsyncdTask) { t.Priority = p } }

// WithInstitution 设置任务归属组织（多租户可见性）
func WithInstitution(id int64) EnqueueOption {
	return func(t *repository.AsyncdTask) { t.InstitutionID = &id }
}

// Schedule 调度项声明（由模块在装配时登记：维护任务、同步器定时、供给重试…）
type Schedule struct {
	JobName         string
	TaskType        string
	Payload         any
	Kind            string // interval | daily
	IntervalSeconds int
	DailyTimes      []string
	WindowMinutes   int
	Enabled         bool
}

// RegisterSchedule 注册/更新调度项（幂等；启动时按代码声明对齐数据库）
func (s *Service) RegisterSchedule(ctx context.Context, schedule Schedule) error {
	if schedule.Kind == "" {
		schedule.Kind = repository.ScheduleKindInterval
	}
	window := schedule.WindowMinutes
	if window <= 0 {
		window = 30
	}
	payload, err := json.Marshal(schedule.Payload)
	if err != nil || schedule.Payload == nil {
		payload = []byte("{}")
	}
	return s.schedules.Upsert(ctx, &repository.AsyncdSchedule{
		JobName: schedule.JobName, TaskType: schedule.TaskType, Payload: payload,
		Kind: schedule.Kind, IntervalSeconds: schedule.IntervalSeconds,
		DailyTimes: schedule.DailyTimes, WindowMinutes: window, Enabled: schedule.Enabled,
	})
}

// Counts 按状态统计任务数（可观测/管理界面）
func (s *Service) Counts(ctx context.Context, taskType string) (map[string]int, error) {
	return s.tasks.Counts(ctx, taskType)
}

// List 列出任务（管理界面）
func (s *Service) List(ctx context.Context, status, taskType string, offset, limit int) ([]*repository.AsyncdTask, error) {
	return s.tasks.List(ctx, status, taskType, offset, limit)
}

// ListSchedules 列出全部调度项
func (s *Service) ListSchedules(ctx context.Context) ([]*repository.AsyncdSchedule, error) {
	return s.schedules.List(ctx)
}

// SetScheduleEnabled 启停调度项
func (s *Service) SetScheduleEnabled(ctx context.Context, jobName string, enabled bool) error {
	return s.schedules.SetEnabled(ctx, jobName, enabled)
}

// RetryArchived 人工重试死信任务
func (s *Service) RetryArchived(ctx context.Context, id int64) error {
	return s.tasks.RetryArchived(ctx, id)
}

// logWarn 统一日志前缀，便于按任务类型排查
func logWarn(msg string, args ...any) { logger.Warn("task "+msg, args...) }
