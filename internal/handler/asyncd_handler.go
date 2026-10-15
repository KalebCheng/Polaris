package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// AsyncdHandler asyncd（任务定义 / 执行实例 / 执行记录 / 执行器实例）管理端点。
//
// 「队列满丢任务」「进程重启后任务卡死」这类问题的解法是**可观测 + 可干预**：
// 这里提供任务目录、某任务的执行记录（含每次尝试的明细）、死信人工重试、
// 定时任务的启停与执行器实例清单。
type AsyncdHandler struct{ svc *asyncd.Service }

// NewAsyncdHandler 创建 asyncd 处理器
func NewAsyncdHandler(svc *asyncd.Service) *AsyncdHandler { return &AsyncdHandler{svc: svc} }

// asyncdScheduleDTO 定时任务的调度规则（异步任务没有这一段）
type asyncdScheduleDTO struct {
	Kind            string     `json:"kind"`
	IntervalSeconds int        `json:"interval_seconds"`
	DailyTimes      []string   `json:"daily_times"`
	WindowMinutes   int        `json:"window_minutes"`
	Enabled         bool       `json:"enabled"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastStatus      string     `json:"last_status"`
}

// asyncdStatsDTO 任务的执行统计
type asyncdStatsDTO struct {
	Total        int            `json:"total"`
	ByStatus     map[string]int `json:"by_status"`
	LastQueuedAt *time.Time     `json:"last_queued_at,omitempty"`
}

// asyncdJobDTO 任务定义（任务中心的一行）：kind=async 时没有 schedule 段。
type asyncdJobDTO struct {
	ID       int64              `json:"id"`
	Name     string             `json:"name"`
	TaskType string             `json:"task_type"`
	Kind     string             `json:"kind"`
	Schedule *asyncdScheduleDTO `json:"schedule,omitempty"`
	Stats    asyncdStatsDTO     `json:"stats"`
}

// asyncdJobRunDTO 执行实例（一次执行）：执行记录列表的一行。
type asyncdJobRunDTO struct {
	ID            int64           `json:"id"`
	JobID         int64           `json:"job_id"`
	JobName       string          `json:"job_name"`
	TaskType      string          `json:"task_type"`
	Status        string          `json:"status"`
	Priority      int             `json:"priority"`
	Attempts      int             `json:"attempts"`
	MaxAttempts   int             `json:"max_attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	UniqueKey     string          `json:"unique_key,omitempty"`
	WorkerID      string          `json:"worker_id,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	InstitutionID *int64          `json:"institution_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
}

// asyncdJobRunAttemptDTO 执行记录（单次尝试）
type asyncdJobRunAttemptDTO struct {
	ID         int64      `json:"id"`
	AttemptNo  int        `json:"attempt_no"`
	WorkerID   string     `json:"worker_id"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMs int        `json:"duration_ms"`
	Error      string     `json:"error,omitempty"`
}

// asyncdWorkerDTO 执行器实例（在线状态由心跳算出）
type asyncdWorkerDTO struct {
	ID              string     `json:"id"`
	InstanceID      string     `json:"instance_id"`
	Host            string     `json:"host"`
	PID             int        `json:"pid"`
	SlotIndex       int        `json:"slot_index"`
	State           string     `json:"state"`
	StartedAt       time.Time  `json:"started_at"`
	LastHeartbeatAt time.Time  `json:"last_heartbeat_at"`
	StoppedAt       *time.Time `json:"stopped_at,omitempty"`
	Running         int        `json:"running"`
}

func toAsyncdJobDTO(item asyncd.JobCatalog) *asyncdJobDTO {
	stats := item.Stats
	byStatus := stats.ByStatus
	if byStatus == nil {
		byStatus = map[string]int{} // nil 集合归一化为空对象，前端不必判空
	}
	dto := &asyncdJobDTO{
		ID: item.Definition.ID, Name: item.Definition.Name, TaskType: item.Definition.TaskType,
		Kind: item.Definition.Kind,
		Stats: asyncdStatsDTO{
			Total: stats.Total, ByStatus: byStatus, LastQueuedAt: stats.LastQueuedAt,
		},
	}
	if item.Definition.Kind == repository.AsyncdJobKindScheduled {
		daily := item.Definition.DailyTimes
		if daily == nil {
			daily = []string{}
		}
		dto.Schedule = &asyncdScheduleDTO{
			Kind: item.Definition.ScheduleKind, IntervalSeconds: item.Definition.IntervalSeconds,
			DailyTimes: daily, WindowMinutes: item.Definition.WindowMinutes,
			Enabled: item.Definition.Enabled, NextRunAt: item.Definition.NextRunAt,
			LastRunAt: item.Definition.LastRunAt, LastStatus: item.Definition.LastStatus,
		}
	}
	return dto
}

func toAsyncdJobRunDTO(view asyncd.JobRunView) *asyncdJobRunDTO {
	jobRun := view.JobRun
	return &asyncdJobRunDTO{
		ID: jobRun.ID, JobID: jobRun.JobID, JobName: view.JobName, TaskType: view.TaskType,
		Status: jobRun.Status, Priority: jobRun.Priority, Attempts: jobRun.Attempts,
		MaxAttempts: jobRun.MaxAttempts, NextAttemptAt: jobRun.NextAttemptAt,
		UniqueKey: jobRun.UniqueKey, WorkerID: jobRun.WorkerID, LastError: jobRun.LastError,
		InstitutionID: jobRun.InstitutionID, Payload: json.RawMessage(jobRun.Payload),
		CreatedAt: jobRun.CreatedAt, UpdatedAt: jobRun.UpdatedAt, FinishedAt: jobRun.FinishedAt,
	}
}

// Jobs 任务目录：所有任务定义（异步 + 定时）及执行统计
func (h *AsyncdHandler) Jobs(c *gin.Context) {
	jobs, err := h.svc.Jobs(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items := make([]*asyncdJobDTO, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, toAsyncdJobDTO(job))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// UpdateJob 启停定时任务（异步任务不可停用）
func (h *AsyncdHandler) UpdateJob(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Enabled == nil {
		bizerrors.Fail(c, bizerrors.New(bizerrors.CodeInvalidParams, "enabled is required"))
		return
	}
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", pathParam(c, "id")))
		return
	}
	if err := h.svc.SetJobEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// JobRuns 列出某任务的执行记录（= 执行实例），并给出与列表同作用域的状态直方图。
// 路径嵌套在任务定义下（/asyncd/jobs/:id/runs）：执行实例必然属于某个定义，
// 归属由路径表达，不再用查询参数（"所有任务的执行"不是本页面的需求）。
func (h *AsyncdHandler) JobRuns(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", pathParam(c, "id")))
		return
	}
	offset, limit, page, size := pageParams(c)
	query := repository.AsyncdJobRunQuery{JobID: id, Status: query(c, "status")}
	views, err := h.svc.JobRuns(c.Request.Context(), query, offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	counts, err := h.svc.JobRunCounts(c.Request.Context(), query)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items := make([]*asyncdJobRunDTO, 0, len(views))
	for _, view := range views {
		items = append(items, toAsyncdJobRunDTO(view))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "counts": counts, "page": page, "size": size})
}

// jobRunInJob 校验"这次执行确实属于路径上的任务定义"，并返回它。
// 嵌套路径的额外好处：拿到一次执行 id 也不能跨任务操作它（越权/误操作直接被挡）。
func (h *AsyncdHandler) jobRunInJob(c *gin.Context, jobID int64) (*asyncd.JobRunView, bool) {
	runID, ok := pathID(c, "runId")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("runId", pathParam(c, "runId")))
		return nil, false
	}
	view, err := h.svc.JobRun(c.Request.Context(), runID)
	if err != nil {
		bizerrors.Fail(c, err)
		return nil, false
	}
	if view.JobRun.JobID != jobID {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("runId", runID))
		return nil, false
	}
	return view, true
}

// JobRunAttempts 某次执行的尝试明细（执行记录）
func (h *AsyncdHandler) JobRunAttempts(c *gin.Context) {
	jobID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", pathParam(c, "id")))
		return
	}
	view, ok := h.jobRunInJob(c, jobID)
	if !ok {
		return
	}
	attempts, err := h.svc.JobRunAttempts(c.Request.Context(), view.JobRun.ID)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items := make([]*asyncdJobRunAttemptDTO, 0, len(attempts))
	for _, attempt := range attempts {
		items = append(items, &asyncdJobRunAttemptDTO{
			ID: attempt.ID, AttemptNo: attempt.AttemptNo, WorkerID: attempt.WorkerID,
			Status: attempt.Status, StartedAt: attempt.StartedAt, FinishedAt: attempt.FinishedAt,
			DurationMs: attempt.DurationMs, Error: attempt.Error,
		})
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// RetryJobRun 人工重试死信执行实例
func (h *AsyncdHandler) RetryJobRun(c *gin.Context) {
	jobID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", pathParam(c, "id")))
		return
	}
	view, ok := h.jobRunInJob(c, jobID)
	if !ok {
		return
	}
	if err := h.svc.RetryDeadJobRun(c.Request.Context(), view.JobRun.ID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"}, http.StatusOK)
}

// Workers 执行器实例清单（在线状态由心跳算出）
func (h *AsyncdHandler) Workers(c *gin.Context) {
	workers, err := h.svc.Workers(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items := make([]*asyncdWorkerDTO, 0, len(workers))
	for _, worker := range workers {
		items = append(items, &asyncdWorkerDTO{
			ID: worker.ID, InstanceID: worker.InstanceID, Host: worker.Host, PID: worker.PID,
			SlotIndex: worker.SlotIndex, State: worker.State, StartedAt: worker.StartedAt,
			LastHeartbeatAt: worker.LastHeartbeatAt, StoppedAt: worker.StoppedAt, Running: worker.Running,
		})
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}
