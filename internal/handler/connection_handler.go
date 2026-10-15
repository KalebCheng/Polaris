package handler

import (
	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/service/connection"
)

// ConnectionHandler 「身份连接」目录处理器：类型、字段 schema、能力与可映射源字段的单一来源。
// 无状态（不依赖仓库），故不注入服务；由 app 装配后挂到路由。
type ConnectionHandler struct{}

// NewConnectionHandler 创建身份连接处理器
func NewConnectionHandler() *ConnectionHandler { return &ConnectionHandler{} }

// Catalog 返回入/出方向的类型目录（前端据此渲染目录卡片与向导表单）。
func (h *ConnectionHandler) Catalog(c *gin.Context) {
	bizerrors.RenderSuccess(c, connection.Build())
}
