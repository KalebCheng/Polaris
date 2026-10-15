package handler

import (
	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/service"
)

// TokenHandler 令牌处理器
type TokenHandler struct{ svc *service.TokenService }

// NewTokenHandler 创建令牌处理器
func NewTokenHandler(svc *service.TokenService) *TokenHandler { return &TokenHandler{svc: svc} }

// List 分页列出令牌
func (h *TokenHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toTokenDTOs(items), "total": total, "page": page, "size": size})
}

// Get 查询令牌
func (h *TokenHandler) Get(c *gin.Context) {
	t, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toTokenDTO(t))
}

// Revoke 吊销令牌
func (h *TokenHandler) Revoke(c *gin.Context) {
	if err := h.svc.Revoke(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除令牌
func (h *TokenHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *TokenHandler) nameByID(c *gin.Context) string {
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
