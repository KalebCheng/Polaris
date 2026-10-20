package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// ProviderHandler 提供商处理器（外部身份提供方 / 邮件 / 短信 / 人机校验 / 存储）
type ProviderHandler struct {
	svc       *service.ProviderService
	templates *service.TemplateService
}

// NewProviderHandler 创建提供商处理器
func NewProviderHandler(svc *service.ProviderService, templates *service.TemplateService) *ProviderHandler {
	return &ProviderHandler{svc: svc, templates: templates}
}

// providerRequest 提供商的对外输入契约（schema 驱动）：
//   - config：非敏感配置，键由类型目录声明；
//   - secrets：敏感明文，键由类型目录声明（留空 = 保持原值）；
//   - policy：SSO 信任策略（仅外部身份提供方）。
//
// 三者都是"未知键即拒绝"，因此不存在"传上来的字段被静默丢弃"。
type providerRequest struct {
	InstitutionID int64             `json:"institution_id"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name"`
	Category      string            `json:"category"`
	Type          string            `json:"type"`
	State         string            `json:"state"`
	Config        map[string]any    `json:"config"`
	Secrets       map[string]string `json:"secrets"`
	Policy        map[string]any    `json:"policy"`
}

func (r providerRequest) toInput(institutionID int64) service.ProviderInput {
	if r.InstitutionID == 0 {
		r.InstitutionID = institutionID
	}
	return service.ProviderInput{
		InstitutionID: r.InstitutionID, Name: r.Name, DisplayName: r.DisplayName,
		Category: r.Category, Type: r.Type, State: r.State,
		Config: r.Config, Secrets: r.Secrets, Policy: r.Policy,
	}
}

// Categories 返回提供商目录（类别 / 类型 / 字段 schema），前端据此渲染目录页与向导。
func (h *ProviderHandler) Categories(c *gin.Context) {
	bizerrors.RenderSuccess(c, h.svc.Catalog())
}

// List 分页列出提供商
func (h *ProviderHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(
		c.Request.Context(), instID(c), query(c, "keyword"), query(c, "category"), offset, limit,
	)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toProviderDTOs(items), "total": total, "page": page, "size": size})
}

// Get 查询提供商
func (h *ProviderHandler) Get(c *gin.Context) {
	item, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toProviderDTO(item))
}

// Create 创建提供商
func (h *ProviderHandler) Create(c *gin.Context) {
	var req providerRequest
	if !bindJSON(c, &req) {
		return
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), req.toInput(instID(c)))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toProviderDTO(created), http.StatusCreated)
}

// Update 更新提供商（路径 id 解析为标识；组织与标识不可变）
func (h *ProviderHandler) Update(c *gin.Context) {
	var req providerRequest
	if !bindJSON(c, &req) {
		return
	}
	name := h.nameByID(c)
	if strings.TrimSpace(name) == "" {
		bizerrors.Fail(c, bizerrors.ErrNotFound.WithDetail("provider", ""))
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), currentUser(c), instID(c), name, req.toInput(instID(c)))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toProviderDTO(updated))
}

type templateRequest struct {
	Name           string   `json:"name"`
	Scene          string   `json:"scene"`
	TemplateCode   string   `json:"template_code"`
	TemplateParams []string `json:"template_params"`
	Subject        string   `json:"subject"`
	Content        string   `json:"content"`
	IsDefault      bool     `json:"is_default"`
}

func (r templateRequest) toInput() service.NotificationTemplateInput {
	return service.NotificationTemplateInput{
		Name: r.Name, Scene: r.Scene, TemplateCode: r.TemplateCode, TemplateParams: r.TemplateParams,
		Subject: r.Subject, Content: r.Content, IsDefault: r.IsDefault,
	}
}

// Templates 列出某提供商的模板（可按 scene 过滤）。
func (h *ProviderHandler) Templates(c *gin.Context) {
	providerID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("provider_id", c.Param("id")))
		return
	}
	items, err := h.templates.List(c.Request.Context(), instID(c), providerID, query(c, "scene"))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// CreateTemplate 新建模板。
func (h *ProviderHandler) CreateTemplate(c *gin.Context) {
	providerID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("provider_id", c.Param("id")))
		return
	}
	var req templateRequest
	if !bindJSON(c, &req) {
		return
	}
	item, err := h.templates.Create(c.Request.Context(), currentUser(c), instID(c), providerID, req.toInput())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, item, http.StatusCreated)
}

// GetTemplate 查询单个模板。
func (h *ProviderHandler) GetTemplate(c *gin.Context) {
	providerID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("provider_id", c.Param("id")))
		return
	}
	templateID, ok := pathID(c, "templateId")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("template_id", c.Param("templateId")))
		return
	}
	item, err := h.templates.Get(c.Request.Context(), instID(c), providerID, templateID)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, item)
}

// UpdateTemplate 更新模板。
func (h *ProviderHandler) UpdateTemplate(c *gin.Context) {
	providerID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("provider_id", c.Param("id")))
		return
	}
	templateID, ok := pathID(c, "templateId")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("template_id", c.Param("templateId")))
		return
	}
	var req templateRequest
	if !bindJSON(c, &req) {
		return
	}
	item, err := h.templates.Update(c.Request.Context(), currentUser(c), instID(c), providerID, templateID, req.toInput())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, item)
}

// DeleteTemplate 删除模板。
func (h *ProviderHandler) DeleteTemplate(c *gin.Context) {
	providerID, ok := pathID(c, "id")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("provider_id", c.Param("id")))
		return
	}
	templateID, ok := pathID(c, "templateId")
	if !ok {
		bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("template_id", c.Param("templateId")))
		return
	}
	if err := h.templates.Delete(c.Request.Context(), currentUser(c), instID(c), providerID, templateID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Test 连通性自检（按类别执行连接/写入探测）
func (h *ProviderHandler) Test(c *gin.Context) {
	if err := h.svc.Test(c.Request.Context(), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetState 只切换启用状态（列表卡片上的快捷开关）
func (h *ProviderHandler) SetState(c *gin.Context) {
	var req struct {
		State string `json:"state"`
	}
	if !bindJSON(c, &req) {
		return
	}
	updated, err := h.svc.SetState(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.State)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toProviderDTO(updated))
}

// Delete 删除提供商
func (h *ProviderHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的标识（对外路径统一用 id，服务层仍以标识语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *ProviderHandler) nameByID(c *gin.Context) string {
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
