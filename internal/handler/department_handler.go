package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// DepartmentHandler 部门处理器
type DepartmentHandler struct{ svc *service.DepartmentService }

// NewDepartmentHandler 创建部门处理器
func NewDepartmentHandler(svc *service.DepartmentService) *DepartmentHandler {
	return &DepartmentHandler{svc: svc}
}

// List 列出当前租户下的部门
func (h *DepartmentHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toDepartmentDTOs(items), "total": total, "page": page, "size": size})
}

// Get 查询部门
func (h *DepartmentHandler) Get(c *gin.Context) {
	d, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toDepartmentDTO(d))
}

// Create 创建部门
func (h *DepartmentHandler) Create(c *gin.Context) {
	var d repository.Department
	if !bindJSON(c, &d) {
		return
	}
	d.InstitutionID = instID(c)
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &d)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toDepartmentDTO(created), http.StatusCreated)
}

// Update 更新部门
func (h *DepartmentHandler) Update(c *gin.Context) {
	var d repository.Department
	if !bindJSON(c, &d) {
		return
	}
	d.InstitutionID = instID(c)
	d.Name = h.nameByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &d); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetMembers 整体替换部门成员
func (h *DepartmentHandler) SetMembers(c *gin.Context) {
	var req struct {
		Users []string `json:"users"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ReplaceMembers(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Users); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除部门
func (h *DepartmentHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Members 列出归属某部门的用户（右侧详情面板成员视图）
func (h *DepartmentHandler) Members(c *gin.Context) {
	items, err := h.svc.Members(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toUserDTOs(items)})
}

// Reorder 同级内上移/下移（body: {"direction":"up|down"}）
func (h *DepartmentHandler) Reorder(c *gin.Context) {
	var req struct {
		Direction string `json:"direction"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.Reorder(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Direction); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *DepartmentHandler) nameByID(c *gin.Context) string {
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
