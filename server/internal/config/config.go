// Package config 從環境變數讀取伺服器設定。
package config

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/s9891326/AIGaming/server/pkg/logger"
)

// Config 是伺服器執行時設定。
type Config struct {
	Port    string // 監聽 port，例如 "8080"
	GinMode string // gin.DebugMode / gin.ReleaseMode / gin.TestMode
	Version string // 版本字串，顯示於 /api/healthz

	LogLevel  slog.Level
	LogFormat logger.Format
	LogColor  logger.ColorMode
}

// Addr 回傳監聽位址，例如 ":8080"。
func (c Config) Addr() string { return ":" + c.Port }

// Load 讀取環境變數，未設定時使用預設值。
func Load() Config {
	return Config{
		Port:      getEnv("PORT", "8080"),
		GinMode:   getEnv("GIN_MODE", gin.DebugMode),
		Version:   getEnv("APP_VERSION", "dev"),
		LogLevel:  logger.ParseLevel(getEnv("LOG_LEVEL", "debug")),
		LogFormat: logger.ParseFormat(getEnv("LOG_FORMAT", string(logger.FormatPretty))),
		LogColor:  logger.ParseColorMode(getEnv("LOG_COLOR", string(logger.ColorAuto))),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
