package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// GroupHandler 用户组处理器
type GroupHandler struct{ svc *service.GroupService }

// NewGroupHandler 创建用户组处理器
func NewGroupHandler(svc *service.GroupService) *GroupHandler { return &GroupHandler{svc: svc} }

// groupListItem 用户组列表项（实体 + 成员数；成员数批量取，避免逐行查询）
type groupListItem struct {
	repository.Group
	MemberCount int `json:"member_count"`
}

// List 分页列出用户组
func (h *GroupHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	counts, err := h.svc.MemberCounts(c.Request.Context(), instID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	out := make([]groupListItem, 0, len(items))
	for _, item := range items {
		out = append(out, groupListItem{Group: *item, MemberCount: counts[item.Name]})
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out, "total": total, "page": page, "size": size})
}

// Detail 用户组详情：成员明细 + 关联角色 + 汇总权限
func (h *GroupHandler) Detail(c *gin.Context) {
	detail, err := h.svc.GetDetail(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	roles := detail.Roles
	if roles == nil {
		roles = []string{}
	}
	permissions := detail.Permissions
	if permissions == nil {
		permissions = []*repository.Permission{}
	}
	bizerrors.RenderSuccess(c, gin.H{
		"group":       detail.Group,
		"members":     toUserDTOs(detail.Members),
		"roles":       roles,
		"permissions": permissions,
	})
}

// Get 查询用户组
func (h *GroupHandler) Get(c *gin.Context) {
	g, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, g)
}

// Create 创建用户组
func (h *GroupHandler) Create(c *gin.Context) {
	var g repository.Group
	if !bindJSON(c, &g) {
		return
	}
	if g.InstitutionID == 0 {
		g.InstitutionID = instID(c)
	}
	g.IsEnabled = true
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &g)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, created, http.StatusCreated)
}

// Update 更新用户组
func (h *GroupHandler) Update(c *gin.Context) {
	var g repository.Group
	if !bindJSON(c, &g) {
		return
	}
	g.InstitutionID = instID(c)
	g.Name = h.nameByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &g); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetMembers 设置用户组成员
func (h *GroupHandler) SetMembers(c *gin.Context) {
	var req struct {
		Users []string `json:"users"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetMembers(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Users); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除用户组
func (h *GroupHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *GroupHandler) nameByID(c *gin.Context) string {
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
