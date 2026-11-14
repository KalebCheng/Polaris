package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// SigninMethodHandler 组织级登录方式处理器。
type SigninMethodHandler struct{ svc *service.SigninMethodService }

// NewSigninMethodHandler 创建组织登录方式处理器。
func NewSigninMethodHandler(svc *service.SigninMethodService) *SigninMethodHandler {
	return &SigninMethodHandler{svc: svc}
}

type signinMethodDTO struct {
	ID               int64     `json:"id"`
	InstitutionID    int64     `json:"institution_id"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"display_name"`
	Identifiers      []string  `json:"identifiers"`
	ProviderID       *int64    `json:"provider_id"`
	EmailTemplateID  *int64    `json:"email_template_id"`
	SMSTemplateID    *int64    `json:"sms_template_id"`
	TTLSeconds       int       `json:"ttl_seconds"`
	MfaEnabled       bool      `json:"mfa_enabled"`
	MfaRequired      bool      `json:"mfa_required"`
	MfaProviderIDs   []int64   `json:"mfa_provider_ids"`
	SortOrder        int       `json:"sort_order"`
	State            string    `json:"state"`
	ApplicationCount int       `json:"application_count"`
	ProviderName     string    `json:"provider_name,omitempty"`
	ProviderDisplay  string    `json:"provider_display_name,omitempty"`
	ProviderState    string    `json:"provider_state,omitempty"`
	TemplateName     string    `json:"template_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toSigninMethodDTO(method *repository.InstitutionSigninMethod) *signinMethodDTO {
	if method == nil {
		return nil
	}
	return &signinMethodDTO{
		ID: method.ID, InstitutionID: method.InstitutionID, Name: method.Name,
		DisplayName: method.DisplayName, Identifiers: orEmptySlice(method.Identifiers),
		ProviderID: method.ProviderID, EmailTemplateID: method.EmailTemplateID, SMSTemplateID: method.SMSTemplateID,
		TTLSeconds: method.TTLSeconds, MfaEnabled: method.MfaEnabled, MfaRequired: method.MfaRequired,
		MfaProviderIDs: orEmptyInt64Slice(method.MfaProviderIDs), SortOrder: method.SortOrder, State: method.State,
		ApplicationCount: method.ApplicationCount, ProviderName: method.ProviderName,
		ProviderDisplay: method.ProviderDisplayName, ProviderState: method.ProviderState,
		TemplateName: method.TemplateName, CreatedAt: method.CreatedAt, UpdatedAt: method.UpdatedAt,
	}
}

// List 列出组织全部登录方式。
func (h *SigninMethodHandler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), pathIDValue(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	out := make([]*signinMethodDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toSigninMethodDTO(item))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out})
}

// Options 返回应用向导可选的启用方法摘要。
func (h *SigninMethodHandler) Options(c *gin.Context) {
	items, err := h.svc.Options(c.Request.Context(), pathIDValue(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	out := make([]*signinMethodDTO, 0, len(items))
	for _, item := range items {
		dto := toSigninMethodDTO(item)
		dto.MfaProviderIDs = []int64{}
		out = append(out, dto)
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out})
}

// Create 创建组织登录方式。
func (h *SigninMethodHandler) Create(c *gin.Context) {
	var in service.SigninMethodInput
	if !bindJSON(c, &in) {
		return
	}
	item, err := h.svc.Create(c.Request.Context(), currentUser(c), pathIDValue(c), in)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toSigninMethodDTO(item), http.StatusCreated)
}

// Update 更新组织登录方式。
func (h *SigninMethodHandler) Update(c *gin.Context) {
	methodID, ok := pathID(c, "methodId")
	if !ok {
		return
	}
	var in service.SigninMethodInput
	if !bindJSON(c, &in) {
		return
	}
	item, err := h.svc.Update(c.Request.Context(), currentUser(c), pathIDValue(c), methodID, in)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toSigninMethodDTO(item))
}

// SetState 启用/禁用组织登录方式。
func (h *SigninMethodHandler) SetState(c *gin.Context) {
	methodID, ok := pathID(c, "methodId")
	if !ok {
		return
	}
	var in struct {
		State string `json:"state" binding:"required"`
	}
	if !bindJSON(c, &in) {
		return
	}
	if err := h.svc.SetState(c.Request.Context(), currentUser(c), pathIDValue(c), methodID, in.State); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除未被应用引用的组织登录方式。
func (h *SigninMethodHandler) Delete(c *gin.Context) {
	methodID, ok := pathID(c, "methodId")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), pathIDValue(c), methodID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}
