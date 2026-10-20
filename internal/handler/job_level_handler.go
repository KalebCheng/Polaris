package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// JobLevelHandler 职级处理器（Spec 第六阶段 Q4：本地 CRUD + 身份源同步；不参与授权）
type JobLevelHandler struct{ svc *service.JobLevelService }

// NewJobLevelHandler 创建职级处理器
func NewJobLevelHandler(svc *service.JobLevelService) *JobLevelHandler {
	return &JobLevelHandler{svc: svc}
}

// nameByID 解析路径 id 为职级标识（对外路径统一用 id，服务层仍以 name 语义访问存储）
func (h *JobLevelHandler) nameByID(c *gin.Context) string {
	name, err := h.svc.NameByID(c.Request.Context(), pathIDValue(c))
	if err != nil {
		return ""
	}
	return name
}

// List 分页列出职级
func (h *JobLevelHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Get 查询职级
func (h *JobLevelHandler) Get(c *gin.Context) {
	name := h.nameByID(c)
	if name == "" {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("job_level", pathIDValue(c)))
		return
	}
	item, err := h.svc.Get(c.Request.Context(), instID(c), name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, item)
}

// Create 创建职级
func (h *JobLevelHandler) Create(c *gin.Context) {
	var l repository.JobLevel
	if !bindJSON(c, &l) {
		return
	}
	if l.InstitutionID == 0 {
		l.InstitutionID = instID(c)
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &l)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, created, http.StatusCreated)
}

// Update 更新职级
func (h *JobLevelHandler) Update(c *gin.Context) {
	var l repository.JobLevel
	if !bindJSON(c, &l) {
		return
	}
	name := h.nameByID(c)
	if name == "" {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("job_level", pathIDValue(c)))
		return
	}
	l.InstitutionID = instID(c)
	l.Name = name
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &l); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"name": name})
}

// Delete 删除职级
func (h *JobLevelHandler) Delete(c *gin.Context) {
	name := h.nameByID(c)
	if name == "" {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("job_level", pathIDValue(c)))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), name); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"name": name})
}

// Distribution 职级分布（组织概览：各级别人数）
func (h *JobLevelHandler) Distribution(c *gin.Context) {
	items, err := h.svc.Distribution(c.Request.Context(), instID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}
