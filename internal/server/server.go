// Package server 提供 HTTP 服务器、路由与中间件。
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"polaris/internal/config"
	"polaris/internal/logger"
)

// Server HTTP 服务器
type Server struct {
	router *gin.Engine
	srv    *http.Server
	cfg    *config.AppConfig
}

// New 创建 HTTP 服务器
func New(cfg *config.AppConfig) *Server {
	switch cfg.Env {
	case "production":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}
	return &Server{router: gin.New(), cfg: cfg}
}

// Router 获取 gin 路由器
func (s *Server) Router() *gin.Engine { return s.router }

// Start 启动服务器（非阻塞）
func (s *Server) Start() error {
	s.srv = &http.Server{
		Addr:           fmt.Sprintf(":%d", s.cfg.Port),
		Handler:        s.router,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	logger.Info("starting http server", "port", s.cfg.Port, "env", s.cfg.Env)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server error", "error", err)
		}
	}()
	return nil
}

// Stop 优雅关闭服务器
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	logger.Info("stopping http server")
	return s.srv.Shutdown(ctx)
}
