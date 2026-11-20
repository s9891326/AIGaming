// Package httpserver 組裝 Gin 路由。
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"aigaming/server/internal/config"
)

// HealthResponse 是 GET /api/healthz 的回應內容。
type HealthResponse struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	ServerTime int64  `json:"serverTime"` // Unix 毫秒
}

// NewRouter 建立所有 HTTP 路由。
// 所有路由都掛在 /api 底下，前端由 Vite（開發）或 nginx（docker）代理過來，同源所以不需要 CORS。
//
// ws 是 WebSocket 端點（GET /ws），為 nil 時不註冊。
func NewRouter(cfg config.Config, log *slog.Logger, ws http.Handler) *gin.Engine {
	gin.SetMode(cfg.GinMode)
	redirectGinDebugOutput(log)

	r := gin.New()
	// 不信任任何代理標頭，ClientIP() 直接使用連線來源位址。
	_ = r.SetTrustedProxies(nil)
	// /ws 的請求要等連線結束才會記錄，而且連線 / 斷線已有專屬日誌，所以降為 Debug。
	r.Use(RequestLogger(log, "/api/healthz", "/ws"), Recovery(log))

	if ws != nil {
		r.GET("/ws", gin.WrapH(ws))
	}

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
