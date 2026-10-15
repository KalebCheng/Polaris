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

// AsyncdHandler asyncd（异步任务与定时调度）管理端点：运维可见性与人工干预。
//
// 「队列满丢任务」「进程重启后任务卡死」这类问题的解法是**可观测 + 可干预**：
// 这里提供按状态计数、任务列表（含死信）与人工重试，以及调度项的启停。
type AsyncdHandler struct{ svc *asyncd.Service }

// NewAsyncdHandler 创建 asyncd 处理器
func NewAsyncdHandler(svc *asyncd.Service) *AsyncdHandler { return &AsyncdHandler{svc: svc} }

// asyncdTaskDTO 任务响应 DTO：payload 解码为对象下发（不再出现 base64 载荷）。
type asyncdTaskDTO struct {
	ID            int64           `json:"id"`
	Type          string          `json:"type"`
	Status        string          `json:"status"`
	Priority      int             `json:"priority"`
	Attempts      int             `json:"attempts"`
	MaxAttempts   int             `json:"max_attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	UniqueKey     string          `json:"unique_key,omitempty"`
	LockedBy      string          `json:"locked_by,omitempty"`
	LastError     string          `json:"last_error,omitempty"`
	InstitutionID *int64          `json:"institution_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
}

func toAsyncdTaskDTO(t *repository.AsyncdTask) *asyncdTaskDTO {
	if t == nil {
		return nil
	}
	return &asyncdTaskDTO{
		ID: t.ID, Type: t.Type, Status: t.Status, Priority: t.Priority,
		Attempts: t.Attempts, MaxAttempts: t.MaxAttempts, NextAttemptAt: t.NextAttemptAt,
		UniqueKey: t.UniqueKey, LockedBy: t.LockedBy, LastError: t.LastError,
		InstitutionID: t.InstitutionID, Payload: json.RawMessage(t.Payload),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, FinishedAt: t.FinishedAt,
	}
}

// Tasks 列出任务与按状态计数
func (h *AsyncdHandler) Tasks(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	status := query(c, "status")
	taskType := query(c, "type")
	items, err := h.svc.List(c.Request.Context(), status, taskType, offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	counts, err := h.svc.Counts(c.Request.Context(), taskType)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	dtos := make([]*asyncdTaskDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, toAsyncdTaskDTO(item))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": dtos, "counts": counts, "page": page, "size": size})
}

// RetryTask 人工重试死信任务
func (h *AsyncdHandler) RetryTask(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", pathParam(c, "id")))
		return
	}
	if err := h.svc.RetryArchived(c.Request.Context(), id); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"}, http.StatusOK)
}

// Schedules 列出调度项
func (h *AsyncdHandler) Schedules(c *gin.Context) {
	items, err := h.svc.ListSchedules(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// UpdateSchedule 启停调度项
func (h *AsyncdHandler) UpdateSchedule(c *gin.Context) {
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
	if err := h.svc.SetScheduleEnabled(c.Request.Context(), pathParam(c, "name"), *req.Enabled); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}
