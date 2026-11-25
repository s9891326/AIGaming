// Package config 從環境變數讀取伺服器設定。
package config

import (
	"log/slog"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"aigaming/server/pkg/logger"
)

// Config 是伺服器執行時設定。
type Config struct {
	Port    string // 監聽 port，例如 "8080"
	GinMode string // gin.DebugMode / gin.ReleaseMode / gin.TestMode
	Version string // 版本字串，顯示於 /api/healthz

	LogLevel  slog.Level
	LogFormat logger.Format
	LogColor  logger.ColorMode

	// WSAllowJSON 允許 WebSocket 以 ?format=json 連線（除錯用）。預設只在非 release 模式開啟。
	WSAllowJSON bool
}

// Addr 回傳監聽位址，例如 ":8080"。
func (c Config) Addr() string { return ":" + c.Port }

// Load 讀取環境變數，未設定時使用預設值。
func Load() Config {
	ginMode := getEnv("GIN_MODE", gin.DebugMode)
	return Config{
		Port:      getEnv("PORT", "8080"),
		GinMode:   ginMode,
		Version:   getEnv("APP_VERSION", "dev"),
		LogLevel:  logger.ParseLevel(getEnv("LOG_LEVEL", "debug")),
		LogFormat: logger.ParseFormat(getEnv("LOG_FORMAT", string(logger.FormatPretty))),
		LogColor:  logger.ParseColorMode(getEnv("LOG_COLOR", string(logger.ColorAuto))),

		WSAllowJSON: getBool("WS_ALLOW_JSON", ginMode != gin.ReleaseMode),
	}
}

// getBool 讀取布林環境變數（1/true/yes/on 或 0/false/no/off），未設定或無法辨識時用預設值。
func getBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
