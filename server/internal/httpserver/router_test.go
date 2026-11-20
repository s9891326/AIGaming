package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/s9891326/AIGaming/server/internal/config"
)

func TestHealthz(t *testing.T) {
	r := NewRouter(config.Config{GinMode: gin.TestMode, Version: "test"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" || body.Version != "test" || body.ServerTime == 0 {
		t.Fatalf("unexpected body: %+v", body)
	}
}
