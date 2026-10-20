package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"polaris/internal/config"
	"polaris/internal/requestmeta"
)

func TestRequestMetadataMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(NewMiddleware(&config.Config{}, nil).RequestMetadata())
	engine.GET("/audit-test", func(c *gin.Context) {
		info := requestmeta.From(c.Request.Context())
		if info.ClientIP != "203.0.113.7" || info.Method != http.MethodGet ||
			info.RequestURI != "/audit-test?source=test" {
			c.JSON(http.StatusInternalServerError, info)
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/audit-test?source=test", nil)
	req.RemoteAddr = "203.0.113.7:54321"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
