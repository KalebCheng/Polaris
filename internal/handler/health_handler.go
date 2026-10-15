package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/pkg/dbclient"
	"polaris/pkg/redisclient"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	db    *dbclient.Client
	redis *redisclient.Client
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(db *dbclient.Client, redis *redisclient.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Liveness 存活探针
func (h *HealthHandler) Liveness(c *gin.Context) {
	bizerrors.RenderSuccess(c, gin.H{"status": "ok", "time": time.Now().Unix()})
}

// Readiness 就绪探针（检查依赖）
func (h *HealthHandler) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	deps := gin.H{}
	ready := true
	if h.db != nil {
		ok, _ := h.db.Health(ctx)
		deps["postgresql"] = ok
		ready = ready && ok
	}
	if h.redis != nil {
		ok, _ := h.redis.Health(ctx)
		deps["redis"] = ok
		ready = ready && ok
	}
	if !ready {
		bizerrors.AbortError(c, bizerrors.New(bizerrors.CodeServiceUnavailable, "dependency unavailable").WithDetail("dependencies", deps))
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ready", "dependencies": deps})
}
