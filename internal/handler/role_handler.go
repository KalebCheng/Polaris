package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// RoleHandler 角色处理器
type RoleHandler struct{ svc *service.RoleService }

// NewRoleHandler 创建角色处理器
func NewRoleHandler(svc *service.RoleService) *RoleHandler { return &RoleHandler{svc: svc} }

// List 分页列出角色
func (h *RoleHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Get 查询角色
func (h *RoleHandler) Get(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", c.Param("id")))
		return
	}
	r, err := h.svc.Get(c.Request.Context(), instID(c), id)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, r)
}

// Create 创建角色
func (h *RoleHandler) Create(c *gin.Context) {
	var r repository.Role
	if !bindJSON(c, &r) {
		return
	}
	if r.InstitutionID == 0 {
		r.InstitutionID = instID(c)
	}
	r.IsEnabled = true
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &r)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, created, http.StatusCreated)
}

// Update 更新角色
func (h *RoleHandler) Update(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", c.Param("id")))
		return
	}
	var r repository.Role
	if !bindJSON(c, &r) {
		return
	}
	r.InstitutionID = instID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), id, &r); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetRelations 设置角色成员/子角色/权限
func (h *RoleHandler) SetRelations(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", c.Param("id")))
		return
	}
	var rel repository.RoleRelations
	if !bindJSON(c, &rel) {
		return
	}
	if err := h.svc.SetRelations(c.Request.Context(), currentUser(c), instID(c), id, &rel); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除角色
func (h *RoleHandler) Delete(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("id", c.Param("id")))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), id); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}
