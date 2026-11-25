package config

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWSAllowJSONDefaultsByGinMode(t *testing.T) {
	tests := []struct {
		ginMode, allowJSON string
		want               bool
	}{
		{gin.DebugMode, "", true},       // 開發：預設允許
		{gin.ReleaseMode, "", false},    // 部署：預設關閉
		{gin.ReleaseMode, "true", true}, // 明確開啟
		{gin.DebugMode, "off", false},   // 明確關閉
		{gin.DebugMode, "maybe", true},  // 無法辨識 → 用預設
	}
	for _, tt := range tests {
		t.Setenv("GIN_MODE", tt.ginMode)
		t.Setenv("WS_ALLOW_JSON", tt.allowJSON)
		if got := Load().WSAllowJSON; got != tt.want {
			t.Errorf("GIN_MODE=%s WS_ALLOW_JSON=%q: got %v, want %v", tt.ginMode, tt.allowJSON, got, tt.want)
		}
	}
}
