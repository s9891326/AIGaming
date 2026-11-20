package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newTestLogger(buf *bytes.Buffer, level slog.Level) *slog.Logger {
	return New(Options{Writer: buf, Level: level, Color: ColorNever})
}

func TestPrettyFormat(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf, slog.LevelDebug)

	log.Info("server listening", "addr", ":8080", "latency", 1500*time.Microsecond)

	got := buf.String()
	for _, want := range []string{" INF ", "server listening", "addr=:8080", "latency=1.5ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "\x1b[") {
		t.Errorf("ColorNever output contains ANSI codes: %q", got)
	}
	if !strings.HasSuffix(got, "\n") || strings.Count(got, "\n") != 1 {
		t.Errorf("want exactly one line, got %q", got)
	}
}

func TestPrettyLevels(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf, slog.LevelInfo)

	log.Debug("hidden")
	log.Info("i")
	log.Warn("w")
	log.Error("e")
	log.Log(t.Context(), slog.LevelInfo+2, "custom")

	got := buf.String()
	if strings.Contains(got, "hidden") {
		t.Errorf("debug record should be filtered: %q", got)
	}
	for _, want := range []string{" INF i", " WRN w", " ERR e", " INF+2 custom"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
}

func TestPrettyAttrsAndGroups(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf, slog.LevelInfo).
		With("svc", "game").
		WithGroup("req").
		With("id", 7)

	log.Info("done",
		"msg", "hello world",
		"empty", "",
		slog.Group("user", "name", "eddy"),
		"err", errors.New("boom"),
	)

	got := buf.String()
	for _, want := range []string{
		"svc=game",
		"req.id=7",
		`req.msg="hello world"`,
		`req.empty=""`,
		"req.user.name=eddy",
		"req.err=boom",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q missing %q", got, want)
		}
	}
}

func TestPrettyColor(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Writer: &buf, Color: ColorAlways})
	log.Warn("careful")
	if !strings.Contains(buf.String(), ansiYellow+"WRN"+ansiReset) {
		t.Errorf("want colored level, got %q", buf.String())
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Writer: &buf, Format: FormatJSON})
	log.Info("hi", "n", 1)

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, buf.String())
	}
	if m["msg"] != "hi" || m["level"] != "INFO" || m["n"] != float64(1) {
		t.Errorf("unexpected JSON: %v", m)
	}
}

func TestParse(t *testing.T) {
	if got := ParseLevel("warn"); got != slog.LevelWarn {
		t.Errorf("ParseLevel(warn) = %v", got)
	}
	if got := ParseLevel("nonsense"); got != slog.LevelInfo {
		t.Errorf("ParseLevel(nonsense) = %v", got)
	}
	if got := ParseFormat("JSON"); got != FormatJSON {
		t.Errorf("ParseFormat(JSON) = %v", got)
	}
	if got := ParseColorMode("always"); got != ColorAlways {
		t.Errorf("ParseColorMode(always) = %v", got)
	}
}
