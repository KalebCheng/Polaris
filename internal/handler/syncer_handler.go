package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
	"time"
)

// SyncerHandler 外部用户数据源处理器
type SyncerHandler struct{ svc *service.SyncerService }

// NewSyncerHandler 创建同步器处理器
func NewSyncerHandler(svc *service.SyncerService) *SyncerHandler { return &SyncerHandler{svc: svc} }

// Types 返回支持的同步器类型
func (h *SyncerHandler) Types(c *gin.Context) {
	bizerrors.RenderSuccess(c, gin.H{"types": service.SyncerTypes()})
}

// List 分页列出同步器
func (h *SyncerHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	dtos := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, toSyncerDTO(item))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": dtos, "total": total, "page": page, "size": size})
}

// Get 查询同步器
func (h *SyncerHandler) Get(c *gin.Context) {
	s, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toSyncerDTO(s))
}

// Create 创建同步器
func (h *SyncerHandler) Create(c *gin.Context) {
	var req struct {
		repository.Syncer
		Secret    string `json:"secret"`
		AppSecret string `json:"app_secret_plain"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.InstitutionID == 0 {
		req.InstitutionID = instID(c)
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), service.SyncerInput{
		Syncer: &req.Syncer, Secret: req.Secret, AppSecret: req.AppSecret,
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toSyncerDTO(created), http.StatusCreated)
}

// Update 更新同步器
func (h *SyncerHandler) Update(c *gin.Context) {
	var req struct {
		repository.Syncer
		Secret    string `json:"secret"`
		AppSecret string `json:"app_secret_plain"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.InstitutionID = instID(c)
	req.Syncer.Name = h.nameByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), service.SyncerInput{
		Syncer: &req.Syncer, Secret: req.Secret, AppSecret: req.AppSecret,
	}); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除同步器
func (h *SyncerHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Run 触发同步：**入队异步执行**（202 + 任务 id），不再让 HTTP 请求等完整轮同步。
//
// 单飞：同一同步器同时只允许一个任务（unique_key = syncer:<id>），
// 因此"运行中再次点击"或被定时器重复触发时不会并发跑同一个数据源。
func (h *SyncerHandler) Run(c *gin.Context) {
	sy, err := h.svc.ByID(c.Request.Context(), instID(c), pathIDValue(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	taskID, existing, err := h.svc.EnqueueRun(c.Request.Context(), sy, service.SyncTriggerManual)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	status := "queued"
	if existing {
		status = "running"
	}
	bizerrors.RenderSuccess(c, gin.H{"task_id": taskID, "status": status}, http.StatusAccepted)
}

// syncerRunDTO 同步运行记录 DTO。
// 实体里的 counts 是 JSONB（[]byte），直接序列化会变成 base64 字符串——
// 这里按 JSON 解码成计数对象后再下发（§3.7：实体一律经 DTO 出网）。
type syncerRunDTO struct {
	ID         int64                  `json:"id"`
	Status     string                 `json:"status"`
	Trigger    string                 `json:"trigger"`
	Message    string                 `json:"message,omitempty"`
	Counts     *repository.SyncCounts `json:"counts,omitempty"`
	Errors     []string               `json:"errors,omitempty"`
	DurationMS int                    `json:"duration_ms"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
}

func toSyncerRunDTO(run *repository.SyncerRun) *syncerRunDTO {
	if run == nil {
		return nil
	}
	dto := &syncerRunDTO{
		ID: run.ID, Status: run.Status, Trigger: run.Trigger, Message: run.Message,
		DurationMS: run.DurationMS, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
	if len(run.Counts) > 0 {
		var counts repository.SyncCounts
		if err := json.Unmarshal(run.Counts, &counts); err == nil {
			dto.Counts = &counts
		}
	}
	if len(run.Errors) > 0 {
		var errs []string
		if err := json.Unmarshal(run.Errors, &errs); err == nil {
			dto.Errors = errs
		}
	}
	return dto
}

// Runs 列出同步明细
func (h *SyncerHandler) Runs(c *gin.Context) {
	offset, limit, _, _ := pageParams(c)
	items, err := h.svc.Runs(c.Request.Context(), instID(c), h.nameByID(c), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	dtos := make([]*syncerRunDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, toSyncerRunDTO(item))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": dtos})
}

// DryRun 试运行：拉取 + 差异 + 同步保护校验，不写任何数据（供管理员确认后再正式同步）。
func (h *SyncerHandler) DryRun(c *gin.Context) {
	preview, err := h.svc.DryRun(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, preview)
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *SyncerHandler) nameByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	name, err := h.svc.NameByID(c.Request.Context(), instID(c), id)
	if err != nil {
		return ""
	}
	return name
}
