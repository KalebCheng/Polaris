package handler

import (
	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// SessionHandler 会话处理器
type SessionHandler struct{ svc *service.SessionService }

// NewSessionHandler 创建会话处理器
func NewSessionHandler(svc *service.SessionService) *SessionHandler { return &SessionHandler{svc: svc} }

// ListByUser 列出用户会话；平台级（scope=platform，仅平台管理员）时跨组织列出全部活动会话。
func (h *SessionHandler) ListByUser(c *gin.Context) {
	user := query(c, "user")
	if user == "" {
		if platformScope(c) {
			items, err := h.svc.AllSessions(c.Request.Context())
			if err != nil {
				bizerrors.Fail(c, err)
				return
			}
			bizerrors.RenderSuccess(c, gin.H{"items": items})
			return
		}
		bizerrors.FailInvalidParams(c, bizerrors.New(bizerrors.CodeInvalidParams, "user is required"))
		return
	}
	items, err := h.svc.UserSessions(c.Request.Context(), instID(c), user)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// Revoke 强制下线指定会话
func (h *SessionHandler) Revoke(c *gin.Context) {
	sessionID := pathParam(c, "sessionID")
	user := query(c, "user")
	if err := h.svc.Revoke(c.Request.Context(), currentUser(c), instID(c), user, sessionID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// RevokeAll 强制下线用户全部会话
func (h *SessionHandler) RevokeAll(c *gin.Context) {
	var req struct {
		User string `json:"user" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.RevokeAll(c.Request.Context(), currentUser(c), instID(c), req.User); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

type selfSessionDTO struct {
	ID            string `json:"id"`
	InstitutionID int64  `json:"institution_id"`
	Name          string `json:"name"`
	Email         string `json:"email,omitempty"`
	Application   string `json:"application,omitempty"`
	IP            string `json:"ip,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	IsCurrent     bool   `json:"is_current"`
}

func toSelfSessionDTO(session *auth.Session, currentSessionID string) selfSessionDTO {
	return selfSessionDTO{
		ID: session.ID, InstitutionID: session.InstitutionID, Name: session.Name, Email: session.Email,
		Application: session.Application, IP: session.IP, UserAgent: session.UserAgent,
		CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt, IsCurrent: session.ID == currentSessionID,
	}
}

// MySessions 列出当前用户自己的活动会话。
func (h *SessionHandler) MySessions(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	items, err := h.svc.OwnSessions(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	out := make([]selfSessionDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toSelfSessionDTO(item, authCtx.SessionID))
	}
	bizerrors.RenderSuccess(c, gin.H{"items": out})
}

// RevokeMySession 撤销当前用户自己的一个会话。
func (h *SessionHandler) RevokeMySession(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	if err := h.svc.RevokeOwnSession(c.Request.Context(), authCtx.InstitutionID, authCtx.Name, pathParam(c, "sessionID")); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// RevokeMySessions 撤销当前用户的全部会话。
func (h *SessionHandler) RevokeMySessions(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	if err := h.svc.RevokeOwnSessions(c.Request.Context(), authCtx.InstitutionID, authCtx.Name); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}
