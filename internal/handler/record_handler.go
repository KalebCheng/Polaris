package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// RecordHandler 审计记录处理器
type RecordHandler struct{ svc *service.RecordService }

// NewRecordHandler 创建审计记录处理器
func NewRecordHandler(svc *service.RecordService) *RecordHandler { return &RecordHandler{svc: svc} }

// List 分页列出审计记录
func (h *RecordHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

type personalRecordDTO struct {
	ID         int64     `json:"id"`
	Method     string    `json:"method,omitempty"`
	RequestURI string    `json:"request_uri,omitempty"`
	Action     string    `json:"action,omitempty"`
	Object     string    `json:"object,omitempty"`
	ClientIP   string    `json:"client_ip,omitempty"`
	StatusCode int       `json:"status_code"`
	CreatedAt  time.Time `json:"created_at"`
}

// MyList 列出当前用户自己的审计事件，不下发管理审计详情或响应内容。
func (h *RecordHandler) MyList(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.ListForUser(c.Request.Context(), authCtx.InstitutionID, authCtx.Name, offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	out := make([]personalRecordDTO, 0, len(items))
	for _, item := range items {
		out = append(out, personalRecordDTO{
			ID: item.ID, Method: item.Method, RequestURI: item.RequestURI, Action: item.Action,
			Object: item.Object, ClientIP: item.ClientIP, StatusCode: item.StatusCode, CreatedAt: item.CreatedAt,
		})
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out, "total": total, "page": page, "size": size})
}

// Get 查询审计记录
func (h *RecordHandler) Get(c *gin.Context) {
	var id int64
	if _, err := fmtSscan(pathParam(c, "id"), &id); err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	r, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, r)
}
