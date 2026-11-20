// Package httpserver 組裝 Gin 路由。
package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/s9891326/AIGaming/server/internal/config"
)

// HealthResponse 是 GET /api/healthz 的回應內容。
type HealthResponse struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	ServerTime int64  `json:"serverTime"` // Unix 毫秒
}

// NewRouter 建立所有 HTTP 路由。
// 所有路由都掛在 /api 底下，前端開發時由 Vite 代理過來，因此不需要 CORS 設定。
func NewRouter(cfg config.Config) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api")
	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{
			Status:     "ok",
			Version:    cfg.Version,
			ServerTime: time.Now().UnixMilli(),
		})
	})

	return r
}
