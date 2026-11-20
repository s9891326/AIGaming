package httpserver

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"aigaming/server/internal/config"
	"aigaming/server/pkg/logger"
)

func newTestRouter(buf *bytes.Buffer) *gin.Engine {
	log := logger.New(logger.Options{Writer: buf, Level: slog.LevelDebug, Color: logger.ColorNever})
	return NewRouter(config.Config{GinMode: gin.TestMode, Version: "test"}, log, nil)
}

func TestHealthz(t *testing.T) {
	var logs bytes.Buffer
	r := newTestRouter(&logs)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))

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

	// 健康檢查會被 docker 頻繁呼叫，成功時只記 Debug。
	if got := logs.String(); !strings.Contains(got, "DBG GET /api/healthz status=200") {
		t.Errorf("want DBG request log for healthz, got %q", got)
	}
}

func TestNotFoundLogsWarn(t *testing.T) {
	var logs bytes.Buffer
	r := newTestRouter(&logs)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/nope", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if got := logs.String(); !strings.Contains(got, "WRN GET /api/nope status=404") {
		t.Errorf("want WRN log for 404, got %q", got)
	}
}

func TestRecovery(t *testing.T) {
	var logs bytes.Buffer
	r := newTestRouter(&logs)
	r.GET("/boom", func(*gin.Context) { panic("kaboom") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	got := logs.String()
	if !strings.Contains(got, "panic recovered") || !strings.Contains(got, "err=kaboom") {
		t.Errorf("panic not logged, got %q", got)
	}
}
