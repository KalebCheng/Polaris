package asyncd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/cron"
)

// Service 任务与调度的对外入口（定义声明 / 入队 / 查询 / 人工干预 / 保留期清理）。
//
// 四维度模型的中心（见 docs/specs/261015-asyncd-task-model.md）：
//   - 任务定义 asyncd_jobs（本服务按代码声明 upsert，运行时按它解析处理器与调度规则）
//   - 执行实例 asyncd_job_runs（入队 → 领取 → 成功/死信）
//   - 执行记录 asyncd_job_run_attempts（每次尝试一行）
//   - 执行器实例 asyncd_workers（注册 + 心跳，在线状态由心跳算出）
//
// 装配方式遵循 AGENTS.md §3.1-1：由 app 统一构造并注入，不做全局单例。
type Service struct {
	jobs        repository.AsyncdJobRepository
	jobRuns     repository.AsyncdJobRunRepository
	runAttempts repository.AsyncdJobRunAttemptRepository
	workers     repository.AsyncdWorkerRepository
	registry    *Registry
	// instanceID 服务实例标识（host-pid）：执行器槽位 id 以它开头
	instanceID string
	clock      func() time.Time
}

// NewService 创建任务服务
func NewService(
	jobs repository.AsyncdJobRepository,
	jobRuns repository.AsyncdJobRunRepository,
	runAttempts repository.AsyncdJobRunAttemptRepository,
	workers repository.AsyncdWorkerRepository,
) *Service {
	return &Service{
		jobs: jobs, jobRuns: jobRuns, runAttempts: runAttempts, workers: workers,
		registry:   NewRegistry(),
		instanceID: instanceID(),
		clock:      time.Now,
	}
}

// instanceID 服务实例标识：主机名 + 进程号（多实例排查"谁领了这个任务"时一眼可辨）
func instanceID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "polaris"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// InstanceID 服务实例标识（执行器登记时写入 asyncd_workers.instance_id）
func (s *Service) InstanceID() string { return s.instanceID }

// Registry 暴露注册表（供 app 装配时登记处理器）
func (s *Service) Registry() *Registry { return s.registry }

// Schedule 任务定义的调度规则（kind=scheduled 时必填）
type Schedule struct {
	// Kind interval | daily | cron
	Kind            string
	IntervalSeconds int
	DailyTimes      []string
	// Cron 标准 5 段表达式（分 时 日 月 周），Kind=cron 时必填
	Cron string
	// WindowMinutes 执行窗口（daily 语义用；默认 30）
	WindowMinutes int
}

// Job 任务定义声明（由代码在启动时给出：名字、处理器绑定、调度规则、载荷模板）
type Job struct {
	Name     string
	TaskType string
	// Kind async | scheduled
	Kind     string
	Schedule Schedule
	// Payload 调度入队时的载荷模板（异步任务由调用方给载荷，这里不用）
	Payload any
	Enabled bool
	// UniqueKey 声明式单飞键（空 = 调度器回落 job:<name>）
	UniqueKey string
}

// RegisterJob 注册/更新任务定义（幂等；启动时按代码声明对齐数据库）。
func (s *Service) RegisterJob(ctx context.Context, job Job) error {
	if job.Kind == "" {
		job.Kind = repository.AsyncdJobKindAsync
	}
	if job.Kind == repository.AsyncdJobKindAsync {
		// 异步任务没有"停用"语义：停用它等于让业务动作静默丢任务，因此恒定启用
		job.Enabled = true
	} else if err := validateSchedule(job.Name, job.Schedule); err != nil {
		return err
	}
	if _, ok := s.registry.Lookup(job.TaskType); !ok {
		return errUnknownTaskType(job.TaskType)
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil || job.Payload == nil {
		payload = []byte("{}")
	}
	return s.jobs.Upsert(ctx, &repository.AsyncdJob{
		Name: job.Name, TaskType: job.TaskType, Kind: job.Kind,
		ScheduleKind: job.Schedule.Kind, IntervalSeconds: job.Schedule.IntervalSeconds,
		DailyTimes: job.Schedule.DailyTimes, CronExpr: job.Schedule.Cron,
		WindowMinutes: windowOrDefault(job.Schedule.WindowMinutes),
		Payload:       payload, Enabled: job.Enabled, UniqueKey: job.UniqueKey,
	})
}

// validateSchedule 校验调度规则的自洽性（代码声明与连接计划共用一处口径）
func validateSchedule(name string, schedule Schedule) error {
	switch schedule.Kind {
	case repository.ScheduleKindInterval:
		if schedule.IntervalSeconds <= 0 {
			return fmt.Errorf("task: 定时任务 %q 的间隔必须为正数", name)
		}
	case repository.ScheduleKindDaily:
		if len(schedule.DailyTimes) == 0 {
			return fmt.Errorf("task: 定时任务 %q 必须声明每天的执行时刻", name)
		}
	case repository.ScheduleKindCron:
		if _, err := cron.Parse(schedule.Cron); err != nil {
			return fmt.Errorf("task: 定时任务 %q 的 cron 表达式非法: %w", name, err)
		}
	case "":
		return fmt.Errorf("task: 定时任务 %q 缺少调度规则", name)
	default:
		return fmt.Errorf("task: 定时任务 %q 的调度规则 %q 不支持", name, schedule.Kind)
	}
	return nil
}

func windowOrDefault(window int) int {
	if window <= 0 {
		return 30
	}
	return window
}

// UpsertJob 写入/更新**业务持有**的任务定义（由持有方在保存/对账时调用），返回任务主键。
//
// 与 RegisterJob（代码声明）的区别：这里规则与启用态以持有方为准、会被覆盖。
// 规则或启用态发生变化时**重置 next_run_at**：否则把"每天 03:00"改成"每 10 分钟"后，
// 最长要等到旧的下次时间才生效——那是又一次"配了不生效"。
//
// asyncd 不记录"谁持有"：归属由调用方保存在自己的资源表上。
func (s *Service) UpsertJob(ctx context.Context, job Job) (int64, error) {
	if err := validateSchedule(job.Name, job.Schedule); err != nil {
		return 0, err
	}
	if _, ok := s.registry.Lookup(job.TaskType); !ok {
		return 0, errUnknownTaskType(job.TaskType)
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil || job.Payload == nil {
		payload = []byte("{}")
	}
	target := &repository.AsyncdJob{
		Name: job.Name, TaskType: job.TaskType, Kind: repository.AsyncdJobKindScheduled,
		ScheduleKind: job.Schedule.Kind, IntervalSeconds: job.Schedule.IntervalSeconds,
		DailyTimes: job.Schedule.DailyTimes, CronExpr: job.Schedule.Cron,
		WindowMinutes: windowOrDefault(job.Schedule.WindowMinutes),
		Payload:       payload, Enabled: job.Enabled, UniqueKey: job.UniqueKey,
	}
	reset := true
	if existing, err := s.jobs.GetByName(ctx, job.Name); err == nil && existing != nil {
		reset = scheduleChanged(existing, target) || (!existing.Enabled && target.Enabled)
	}
	return s.jobs.UpsertJob(ctx, target, reset)
}

// scheduleChanged 判断"规则是否变了"（决定要不要重置 next_run_at）
func scheduleChanged(existing, target *repository.AsyncdJob) bool {
	if existing.ScheduleKind != target.ScheduleKind ||
		existing.IntervalSeconds != target.IntervalSeconds ||
		existing.CronExpr != target.CronExpr ||
		existing.WindowMinutes != target.WindowMinutes ||
		existing.UniqueKey != target.UniqueKey {
		return true
	}
	if len(existing.DailyTimes) != len(target.DailyTimes) {
		return true
	}
	for i, at := range existing.DailyTimes {
		if at != target.DailyTimes[i] {
			return true
		}
	}
	return string(existing.Payload) != string(target.Payload)
}

// GetByID 按主键取任务定义（业务持有方对账时用：校验自己记录的任务 id 是否还存在）。
func (s *Service) GetByID(ctx context.Context, id int64) (*repository.AsyncdJob, error) {
	return s.jobs.GetByID(ctx, id)
}

// SetEnabledByName 按任务名启停（业务持有方的计划变化 / 停用时调用）
func (s *Service) SetEnabledByName(ctx context.Context, name string, enabled bool) error {
	return s.jobs.SetEnabledByName(ctx, name, enabled)
}

// RetireUndeclared 退役"本次启动未声明且无归属"的定义（保留历史执行记录）。
// 代码里删掉一个任务声明时走这条路，而不是 DELETE（删定义会级联删掉执行历史）。
func (s *Service) RetireUndeclared(ctx context.Context, activeNames []string) (int64, error) {
	return s.jobs.RetireUndeclared(ctx, activeNames, s.clock())
}

// Enqueue 入队一个执行实例。
//   - jobName：任务定义名（决定处理器、归属与执行记录的分组）；
//   - payload：任意可 JSON 序列化的对象（处理器自行按 payloadVersion 兼容）；
//   - uniqueKey：单飞键（同一键在 queued/running 下唯一）；传空表示不去重；
//   - runAt：延迟执行时间（零值 = 立即）。
//
// 返回 existing=true 表示撞上单飞键、复用了已存在的实例（不会重复执行）。
// 定义不存在或未注册处理器时**拒绝入队**：避免"入了队却没人执行"的静默堆积。
func (s *Service) Enqueue(
	ctx context.Context, jobName string, payload any, uniqueKey string, runAt time.Time, opts ...EnqueueOption,
) (id int64, existing bool, err error) {
	job, err := s.jobs.GetByName(ctx, jobName)
	if err != nil {
		return 0, false, errJobNotFound(jobName)
	}
	if _, ok := s.registry.Lookup(job.TaskType); !ok {
		return 0, false, errUnknownTaskType(job.TaskType)
	}
	jobRun := &repository.AsyncdJobRun{
		JobID: job.ID, PayloadVersion: 1, MaxAttempts: DefaultMaxAttempts,
		UniqueKey: uniqueKey, NextAttemptAt: runAt,
	}
	for _, opt := range opts {
		opt(jobRun)
	}
	if raw, marshalErr := json.Marshal(payload); marshalErr == nil && payload != nil {
		jobRun.Payload = raw
	}
	return s.jobRuns.Enqueue(ctx, jobRun)
}

// EnqueueOption 入队选项
type EnqueueOption func(*repository.AsyncdJobRun)

// WithMaxAttempts 设置最大尝试次数
func WithMaxAttempts(n int) EnqueueOption {
	return func(r *repository.AsyncdJobRun) {
		if n > 0 {
			r.MaxAttempts = n
		}
	}
}

// WithDeadline 设置任务级截止时间（超期不再领取）
func WithDeadline(at time.Time) EnqueueOption {
	return func(r *repository.AsyncdJobRun) { r.Deadline = &at }
}

// WithPriority 设置优先级（越大越先被领取）
func WithPriority(p int) EnqueueOption { return func(r *repository.AsyncdJobRun) { r.Priority = p } }

// WithInstitution 设置执行实例归属组织（多租户可见性）
func WithInstitution(id int64) EnqueueOption {
	return func(r *repository.AsyncdJobRun) { r.InstitutionID = &id }
}

// JobStats 一个任务的执行统计（来自 asyncd_job_runs 的聚合）
type JobStats struct {
	Total    int
	ByStatus map[string]int
	// LastQueuedAt 最近一次入队时间（还没执行过 = nil）
	LastQueuedAt *time.Time
}

// JobCatalog 任务目录条目：定义 + 执行统计
type JobCatalog struct {
	Definition *repository.AsyncdJob
	Stats      JobStats
}

// Jobs 任务目录：所有已登记的任务定义（异步 + 定时）及各自的执行统计。
//
// 定义表是目录的唯一来源，因此**从未执行过的任务也在列**——
// 只看执行实例会把"没跑过"显示成"任务不存在"。
func (s *Service) Jobs(ctx context.Context) ([]JobCatalog, error) {
	definitions, err := s.jobs.List(ctx, false)
	if err != nil {
		return nil, err
	}
	stats, err := s.jobRuns.Stats(ctx)
	if err != nil {
		return nil, err
	}
	byJob := make(map[int64]JobStats, len(definitions))
	for _, row := range stats {
		item := byJob[row.JobID]
		if item.ByStatus == nil {
			item.ByStatus = map[string]int{}
		}
		item.Total += row.Count
		item.ByStatus[row.Status] += row.Count
		if item.LastQueuedAt == nil || row.LastQueuedAt.After(*item.LastQueuedAt) {
			at := row.LastQueuedAt
			item.LastQueuedAt = &at
		}
		byJob[row.JobID] = item
	}
	out := make([]JobCatalog, 0, len(definitions))
	for _, definition := range definitions {
		item := byJob[definition.ID]
		if item.ByStatus == nil {
			item.ByStatus = map[string]int{}
		}
		out = append(out, JobCatalog{Definition: definition, Stats: item})
	}
	return out, nil
}

// SetJobEnabled 启停任务定义（按主键，与对外路径的资源标识约定一致）。
// 只有定时任务可以停用：异步任务若被停用就变成"静默丢任务"（AGENTS §8 队列丢任务那行的教训）。
func (s *Service) SetJobEnabled(ctx context.Context, id int64, enabled bool) error {
	job, err := s.jobs.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if job.Kind != repository.AsyncdJobKindScheduled {
		return errors.ErrInvalidParams.WithDetail("id", id).WithDetail("kind", job.Kind)
	}
	return s.jobs.SetEnabled(ctx, id, enabled)
}

// JobRunCounts 按状态统计执行实例数（可观测/管理界面）
func (s *Service) JobRunCounts(ctx context.Context, query repository.AsyncdJobRunQuery) (map[string]int, error) {
	return s.jobRuns.Counts(ctx, query)
}

// JobRunView 执行实例 + 归属信息（界面上"哪个任务的哪一次执行"）
type JobRunView struct {
	JobRun   *repository.AsyncdJobRun
	JobName  string
	TaskType string
}

// Runs 列出执行实例（= 界面上的执行记录列表），并带上归属的任务定义
func (s *Service) JobRuns(ctx context.Context, query repository.AsyncdJobRunQuery, offset, limit int) ([]JobRunView, error) {
	claimed, err := s.jobRuns.List(ctx, query, offset, limit)
	if err != nil {
		return nil, err
	}
	index, err := s.jobIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]JobRunView, 0, len(claimed))
	for _, jobRun := range claimed {
		view := JobRunView{JobRun: jobRun}
		if job, ok := index[jobRun.JobID]; ok {
			view.JobName = job.Name
			view.TaskType = job.TaskType
		}
		out = append(out, view)
	}
	return out, nil
}

// Run 按主键取执行实例（含归属）
func (s *Service) JobRun(ctx context.Context, id int64) (*JobRunView, error) {
	jobRun, err := s.jobRuns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	view := &JobRunView{JobRun: jobRun}
	index, err := s.jobIndex(ctx)
	if err != nil {
		return nil, err
	}
	if job, ok := index[jobRun.JobID]; ok {
		view.JobName = job.Name
		view.TaskType = job.TaskType
	}
	return view, nil
}

// jobIndex 任务定义索引（定义是小集合，一次读回比逐个查更划算；缺定义时归属留空即可）
func (s *Service) jobIndex(ctx context.Context) (map[int64]*repository.AsyncdJob, error) {
	// 含退役定义：退役任务的执行记录仍要显示"这是哪个任务"
	definitions, err := s.jobs.List(ctx, true)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]*repository.AsyncdJob, len(definitions))
	for _, job := range definitions {
		index[job.ID] = job
	}
	return index, nil
}

// JobRunAttempts 取某次执行的全部尝试（执行记录明细）
func (s *Service) JobRunAttempts(ctx context.Context, jobRunID int64) ([]*repository.AsyncdJobRunAttempt, error) {
	if _, err := s.jobRuns.GetByID(ctx, jobRunID); err != nil {
		return nil, err
	}
	return s.runAttempts.ListByRun(ctx, jobRunID)
}

// RetryDead 人工重试死信实例（回到 queued 并清零尝试计数）
func (s *Service) RetryDeadJobRun(ctx context.Context, id int64) error {
	jobRun, err := s.jobRuns.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if jobRun.Status != repository.AsyncdJobRunDead {
		return errors.ErrInvalidParams.WithDetail("status", jobRun.Status)
	}
	return s.jobRuns.RetryDead(ctx, id)
}

// WorkerInfo 执行器实例的对外视图（在线状态**计算得出**，不落库）。
// 与 Worker（执行器本体）区分：这里是"给人看的快照"。
type WorkerInfo struct {
	ID         string
	InstanceID string
	Host       string
	PID        int
	SlotIndex  int
	// State online（心跳新鲜）| stale（心跳过期，疑似崩溃）| stopped（优雅退出）
	State           string
	StartedAt       time.Time
	LastHeartbeatAt time.Time
	StoppedAt       *time.Time
	// Running 该槽位当前持租约在执行的任务数
	Running int
}

// 执行器在线状态（由心跳算出，避免派生列与心跳漂移）
const (
	WorkerStateOnline  = "online"
	WorkerStateStale   = "stale"
	WorkerStateStopped = "stopped"
)

// Workers 执行器实例清单（含在线状态与在跑任务数）
func (s *Service) Workers(ctx context.Context) ([]WorkerInfo, error) {
	rows, err := s.workers.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	out := make([]WorkerInfo, 0, len(rows))
	for _, row := range rows {
		state := WorkerStateOnline
		switch {
		case row.Worker.StoppedAt != nil:
			state = WorkerStateStopped
		case now.Sub(row.Worker.LastHeartbeatAt) > WorkerStaleAfter:
			state = WorkerStateStale
		}
		out = append(out, WorkerInfo{
			ID: row.Worker.ID, InstanceID: row.Worker.InstanceID, Host: row.Worker.Host,
			PID: row.Worker.PID, SlotIndex: row.Worker.SlotIndex, State: state,
			StartedAt: row.Worker.StartedAt, LastHeartbeatAt: row.Worker.LastHeartbeatAt,
			StoppedAt: row.Worker.StoppedAt, Running: row.Running,
		})
	}
	return out, nil
}

// Purge 保留期清理：删终态且完成时间早于 runsBefore 的执行实例（尝试记录随外键级联删除），
// 并清理停止/失联早于 workersBefore 的执行器行。
func (s *Service) Purge(ctx context.Context, jobRunsBefore, workersBefore time.Time, limit int) (int64, int64, error) {
	deletedRuns, err := s.jobRuns.PurgeFinishedBefore(ctx, jobRunsBefore, limit)
	if err != nil {
		return 0, 0, err
	}
	deletedWorkers, err := s.workers.PurgeStaleBefore(ctx, workersBefore)
	if err != nil {
		return deletedRuns, 0, err
	}
	return deletedRuns, deletedWorkers, nil
}

// workerRecord 构造执行器槽位的登记信息（id = <host>-<pid>#<slot>，与执行实例的 worker_id 同源）
func (s *Service) workerRecord(slotIndex int) *repository.AsyncdWorker {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "polaris"
	}
	return &repository.AsyncdWorker{
		ID:         fmt.Sprintf("%s#%d", s.instanceID, slotIndex),
		InstanceID: s.instanceID,
		Host:       host,
		PID:        os.Getpid(),
		SlotIndex:  slotIndex,
	}
}

// logWarn 统一日志前缀，便于按任务类型排查
func logWarn(msg string, args ...any) { logger.Warn("task "+msg, args...) }
