package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// JobTitleHandler 职务处理器
type JobTitleHandler struct{ svc *service.JobTitleService }

// NewJobTitleHandler 创建职务处理器
func NewJobTitleHandler(svc *service.JobTitleService) *JobTitleHandler {
	return &JobTitleHandler{svc: svc}
}

// jobTitleListItem 职务列表项（实体 + 任职人数；人数批量取，避免逐行查询）
type jobTitleListItem struct {
	repository.JobTitle
	MemberCount int `json:"member_count"`
}

// jobTitleDetailDTO 职务详情响应：顶层键与前端类型保持一致（jobTitle，而不是历史遗留的 group）。
// members 必须走 toUserDTOs，避免把密码哈希 / MFA 密钥等实体字段直接下发。
type jobTitleDetailDTO struct {
	JobTitle    *repository.JobTitle     `json:"jobTitle"`
	Members     []*userDTO               `json:"members"`
	Roles       []string                 `json:"roles"`
	Permissions []*repository.Permission `json:"permissions"`
}

// List 分页列出职务
func (h *JobTitleHandler) List(c *gin.Context) {
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
	out := make([]jobTitleListItem, 0, len(items))
	for _, item := range items {
		out = append(out, jobTitleListItem{JobTitle: *item, MemberCount: counts[item.Name]})
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out, "total": total, "page": page, "size": size})
}

// Detail 职务详情：任职人员明细 + 关联角色 + 汇总权限
func (h *JobTitleHandler) Detail(c *gin.Context) {
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
	members := toUserDTOs(detail.Members)
	if members == nil {
		members = []*userDTO{}
	}
	bizerrors.RenderSuccess(c, jobTitleDetailDTO{
		JobTitle:    detail.JobTitle,
		Members:     members,
		Roles:       roles,
		Permissions: permissions,
	})
}

// Get 查询职务
func (h *JobTitleHandler) Get(c *gin.Context) {
	g, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, g)
}

// Create 创建职务
func (h *JobTitleHandler) Create(c *gin.Context) {
	var g repository.JobTitle
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

// Update 更新职务
func (h *JobTitleHandler) Update(c *gin.Context) {
	var g repository.JobTitle
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

// SetMembers 设置职务成员
func (h *JobTitleHandler) SetMembers(c *gin.Context) {
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

// Delete 删除职务
func (h *JobTitleHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *JobTitleHandler) nameByID(c *gin.Context) string {
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
