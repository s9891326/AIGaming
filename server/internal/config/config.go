// Package config 從環境變數讀取伺服器設定。
package config

import (
	"os"

	"github.com/gin-gonic/gin"
)

// Config 是伺服器執行時設定。
type Config struct {
	Addr    string // 監聽位址，例如 ":8080"
	GinMode string // gin.DebugMode / gin.ReleaseMode / gin.TestMode
	Version string // 版本字串，顯示於 /api/healthz
}

// Load 讀取環境變數，未設定時使用預設值。
func Load() Config {
	return Config{
		Addr:    ":" + getEnv("PORT", "8080"),
		GinMode: getEnv("GIN_MODE", gin.DebugMode),
		Version: getEnv("APP_VERSION", "dev"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
