package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// InstitutionHandler 组织处理器
type InstitutionHandler struct{ svc *service.InstitutionService }

type institutionInput struct {
	repository.Institution
	DefaultPassword string `json:"default_password"`
	PasswordSalt    string `json:"password_salt"`
}

func (in *institutionInput) entity() *repository.Institution {
	org := in.Institution
	org.DefaultPasswordPlain = strings.TrimSpace(in.DefaultPassword)
	org.PasswordSalt = strings.TrimSpace(in.PasswordSalt)
	return &org
}

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
	var in institutionInput
	if !bindJSON(c, &in) {
		return
	}
	org := in.entity()
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), org)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toInstitutionDTO(created), http.StatusCreated)
}

// Update 更新组织
func (h *InstitutionHandler) Update(c *gin.Context) {
	var in institutionInput
	if !bindJSON(c, &in) {
		return
	}
	org := in.entity()
	org.ID = pathIDValue(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), org); err != nil {
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
