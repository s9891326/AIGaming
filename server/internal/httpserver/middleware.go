package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"aigaming/server/pkg/logger"
)

// RequestLogger 用專案的 logger 取代 gin.Logger()，每個請求印一行：
//
//	INF GET /api/healthz status=200 latency=152µs ip=::1
//
// 依狀態碼決定等級：5xx → Error、4xx → Warn、其他 → Info。
// quietPaths 中的路徑（例如健康檢查）成功時降為 Debug，避免洗版。
func RequestLogger(log *slog.Logger, quietPaths ...string) gin.HandlerFunc {
	quiet := make(map[string]bool, len(quietPaths))
	for _, p := range quietPaths {
		quiet[p] = true
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if q := c.Request.URL.RawQuery; q != "" {
			path += "?" + q
		}

		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		case quiet[c.Request.URL.Path]:
			level = slog.LevelDebug
		}

		attrs := []slog.Attr{
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("ip", c.ClientIP()),
		}
		if errs := c.Errors.ByType(gin.ErrorTypePrivate).String(); errs != "" {
			attrs = append(attrs, slog.String("err", strings.TrimSpace(errs)))
		}
		logger.LogNoSource(c.Request.Context(), log, level, c.Request.Method+" "+path, attrs...)
	}
}

// Recovery 攔截 handler 的 panic，記錄錯誤與堆疊後回傳 500，取代 gin.Recovery()。
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered",
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"err", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// redirectGinDebugOutput 讓 gin 的 [GIN-debug] 訊息（路由表、警告）也走專案 logger。
func redirectGinDebugOutput(log *slog.Logger) {
	gin.DebugPrintRouteFunc = func(method, path, handler string, _ int) {
		logger.LogNoSource(context.Background(), log, slog.LevelDebug, "route registered",
			slog.String("method", method),
			slog.String("path", path),
			// "github.com/.../httpserver.NewRouter.func1" → "httpserver.NewRouter.func1"
			slog.String("handler", handler[strings.LastIndexByte(handler, '/')+1:]),
		)
	}
	gin.DebugPrintFunc = func(format string, values ...any) {
		msg := strings.TrimSpace(fmt.Sprintf(format, values...))
		msg = strings.TrimPrefix(msg, "[WARNING] ")
		// gin 的警告訊息常是多行說明，只保留第一行。
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		logger.LogNoSource(context.Background(), log, slog.LevelDebug, "gin: "+msg)
	}
}
