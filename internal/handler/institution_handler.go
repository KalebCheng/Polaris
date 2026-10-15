package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// InstitutionHandler 组织处理器
type InstitutionHandler struct{ svc *service.InstitutionService }

// NewInstitutionHandler 创建组织处理器
func NewInstitutionHandler(svc *service.InstitutionService) *InstitutionHandler {
	return &InstitutionHandler{svc: svc}
}

// List 分页列出组织
func (h *InstitutionHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toInstitutionDTOs(items), "total": total, "page": page, "size": size})
}

// Get 查询组织
func (h *InstitutionHandler) Get(c *gin.Context) {
	org, err := h.svc.Get(c.Request.Context(), pathIDValue(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toInstitutionDTO(org))
}

// Create 创建组织
func (h *InstitutionHandler) Create(c *gin.Context) {
	var org repository.Institution
	if !bindJSON(c, &org) {
		return
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &org)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toInstitutionDTO(created), http.StatusCreated)
}

// Update 更新组织
func (h *InstitutionHandler) Update(c *gin.Context) {
	var org repository.Institution
	if !bindJSON(c, &org) {
		return
	}
	org.ID = pathIDValue(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &org); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Stats 组织资源聚合统计
func (h *InstitutionHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats(c.Request.Context(), pathIDValue(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, stats)
}

// StatsAll 全部组织的聚合统计（总览页按组织聚合）
func (h *InstitutionHandler) StatsAll(c *gin.Context) {
	stats, err := h.svc.StatsAll(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": stats})
}

// Delete 删除组织
func (h *InstitutionHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), pathIDValue(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *InstitutionHandler) nameByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	name, err := h.svc.NameByID(c.Request.Context(), id)
	if err != nil {
		return ""
	}
	return name
}
