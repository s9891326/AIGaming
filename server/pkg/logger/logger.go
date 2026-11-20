// Package logger 提供專案統一的 slog.Logger。
//
// 開發時使用 Pretty 格式（彩色、精簡、易讀）：
//
//	17:03:44.123 INF main.go:31 server listening addr=:8080 mode=debug
//
// 部署（docker）時可切換成 JSON 格式，方便日誌系統收集。
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Format 是輸出格式。
type Format string

const (
	FormatPretty Format = "pretty"
	FormatJSON   Format = "json"
)

// ColorMode 決定 Pretty 格式是否輸出 ANSI 顏色。
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"   // 輸出到終端機時才上色
	ColorAlways ColorMode = "always" // 一律上色（例如 docker logs、task 並行輸出）
	ColorNever  ColorMode = "never"
)

// Options 是建立 Logger 的設定。零值可直接使用（Info 等級、Pretty、stdout、自動顏色）。
type Options struct {
	Level     slog.Leveler
	Format    Format
	Color     ColorMode
	Writer    io.Writer
	AddSource bool // 是否附上呼叫位置（檔名:行號）
}

// New 依設定建立 Logger。
func New(opts Options) *slog.Logger {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Level == nil {
		opts.Level = slog.LevelInfo
	}

	switch opts.Format {
	case FormatJSON:
		return slog.New(slog.NewJSONHandler(opts.Writer, &slog.HandlerOptions{
			Level:     opts.Level,
			AddSource: opts.AddSource,
		}))
	default:
		return slog.New(NewPrettyHandler(opts.Writer, &PrettyOptions{
			Level:     opts.Level,
			AddSource: opts.AddSource,
			Color:     useColor(opts.Color, opts.Writer),
		}))
	}
}

// LogNoSource 記錄一筆不帶呼叫位置的紀錄。
// 適用於 middleware、轉接器等「固定從同一行發出」的日誌，這時印出來源只是雜訊。
func LogNoSource(ctx context.Context, l *slog.Logger, level slog.Level, msg string, attrs ...slog.Attr) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !l.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0) // pc = 0 → 無來源
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}

// ParseLevel 將 "debug" / "info" / "warn" / "error" 轉為 slog.Level，無法辨識時回傳 Info。
func ParseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.TrimSpace(s))); err != nil {
		return slog.LevelInfo
	}
	return l
}

// ParseFormat 將字串轉為 Format，無法辨識時回傳 Pretty。
func ParseFormat(s string) Format {
	if Format(strings.ToLower(strings.TrimSpace(s))) == FormatJSON {
		return FormatJSON
	}
	return FormatPretty
}

// ParseColorMode 將字串轉為 ColorMode，無法辨識時回傳 Auto。
func ParseColorMode(s string) ColorMode {
	switch m := ColorMode(strings.ToLower(strings.TrimSpace(s))); m {
	case ColorAlways, ColorNever:
		return m
	default:
		return ColorAuto
	}
}
