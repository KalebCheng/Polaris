package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// PermissionHandler 权限处理器
type PermissionHandler struct{ svc *service.PermissionService }

// NewPermissionHandler 创建权限处理器
func NewPermissionHandler(svc *service.PermissionService) *PermissionHandler {
	return &PermissionHandler{svc: svc}
}

// List 分页列出权限
func (h *PermissionHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Get 查询权限
func (h *PermissionHandler) Get(c *gin.Context) {
	p, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, p)
}

// Create 创建权限
func (h *PermissionHandler) Create(c *gin.Context) {
	var p repository.Permission
	if !bindJSON(c, &p) {
		return
	}
	if p.InstitutionID == 0 {
		p.InstitutionID = instID(c)
	}
	p.IsEnabled = true
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &p)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, created, http.StatusCreated)
}

// Update 更新权限
func (h *PermissionHandler) Update(c *gin.Context) {
	var p repository.Permission
	if !bindJSON(c, &p) {
		return
	}
	p.InstitutionID = instID(c)
	p.Name = h.nameByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &p); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetSubjects 设置权限主体
func (h *PermissionHandler) SetSubjects(c *gin.Context) {
	var req struct {
		Users  []string `json:"users"`
		Groups []string `json:"groups"`
		Roles  []string `json:"roles"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetSubjects(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Users, req.Groups, req.Roles); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除权限
func (h *PermissionHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Enforce 判定用户是否可对资源执行动作
func (h *PermissionHandler) Enforce(c *gin.Context) {
	var req struct {
		InstitutionID int64  `json:"institution_id"`
		User          string `json:"user"`
		Resource      string `json:"resource"`
		Action        string `json:"action"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.InstitutionID == 0 {
		req.InstitutionID = instID(c)
	}
	allowed, roles, err := h.svc.Enforce(c.Request.Context(), req.InstitutionID, req.User, req.Resource, req.Action)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"allowed": allowed, "roles": roles})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *PermissionHandler) nameByID(c *gin.Context) string {
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
