package logger

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unicode"
)

// ANSI 顏色碼。
const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiBoldRed = "\x1b[1;31m"
)

// PrettyOptions 是 PrettyHandler 的設定。
type PrettyOptions struct {
	Level     slog.Leveler
	AddSource bool
	Color     bool
}

// PrettyHandler 是給人看的 slog.Handler，一筆紀錄一行：
//
//	時間 等級 [來源] 訊息 key=value ...
type PrettyHandler struct {
	opts   PrettyOptions
	mu     *sync.Mutex // 由所有衍生的 handler 共用，確保同一個 writer 不會交錯寫入
	w      io.Writer
	attrs  []byte // WithAttrs 預先格式化好的屬性
	prefix string // WithGroup 累積的群組前綴，例如 "req."
}

// NewPrettyHandler 建立 PrettyHandler。opts 可為 nil。
func NewPrettyHandler(w io.Writer, opts *PrettyOptions) *PrettyHandler {
	h := &PrettyHandler{mu: &sync.Mutex{}, w: w}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = slog.LevelInfo
	}
	return h
}

// Enabled 實作 slog.Handler。
func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

// Handle 實作 slog.Handler。
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	buf := make([]byte, 0, 256)

	if !r.Time.IsZero() {
		buf = h.paint(buf, ansiDim, r.Time.Format("15:04:05.000"))
		buf = append(buf, ' ')
	}

	buf = h.appendLevel(buf, r.Level)
	buf = append(buf, ' ')

	if h.opts.AddSource && r.PC != 0 {
		f, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		if f.File != "" {
			buf = h.paint(buf, ansiDim, filepath.Base(f.File)+":"+strconv.Itoa(f.Line))
			buf = append(buf, ' ')
		}
	}

	buf = append(buf, r.Message...)
	buf = append(buf, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		buf = h.appendAttr(buf, h.prefix, a)
		return true
	})
	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

// WithAttrs 實作 slog.Handler。
func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()
	for _, a := range attrs {
		h2.attrs = h2.appendAttr(h2.attrs, h2.prefix, a)
	}
	return h2
}

// WithGroup 實作 slog.Handler。
func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.prefix += name + "."
	return h2
}

func (h *PrettyHandler) clone() *PrettyHandler {
	h2 := *h
	h2.attrs = append([]byte(nil), h.attrs...)
	return &h2
}

func (h *PrettyHandler) appendLevel(buf []byte, l slog.Level) []byte {
	switch {
	case l >= slog.LevelError:
		return h.paint(buf, ansiBoldRed, levelText(l, "ERR", slog.LevelError))
	case l >= slog.LevelWarn:
		return h.paint(buf, ansiYellow, levelText(l, "WRN", slog.LevelWarn))
	case l >= slog.LevelInfo:
		return h.paint(buf, ansiGreen, levelText(l, "INF", slog.LevelInfo))
	default:
		return h.paint(buf, ansiMagenta, levelText(l, "DBG", slog.LevelDebug))
	}
}

// levelText 回傳三個字的等級縮寫；自訂等級（例如 Info+2）會顯示成 "INF+2"。
func levelText(l slog.Level, name string, base slog.Level) string {
	if l == base {
		return name
	}
	return fmt.Sprintf("%s%+d", name, int(l-base))
}

func (h *PrettyHandler) appendAttr(buf []byte, prefix string, a slog.Attr) []byte {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return buf
	}

	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		if len(group) == 0 {
			return buf
		}
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, ga := range group {
			buf = h.appendAttr(buf, p, ga)
		}
		return buf
	}

	buf = append(buf, ' ')
	buf = h.paint(buf, ansiCyan, prefix+a.Key+"=")

	val := formatValue(a.Value)
	if _, isErr := a.Value.Any().(error); isErr || a.Key == "err" || a.Key == "error" {
		return h.paint(buf, ansiRed, val)
	}
	return append(buf, val...)
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return quoteIfNeeded(v.String())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case error:
			return quoteIfNeeded(x.Error())
		case fmt.Stringer:
			return quoteIfNeeded(x.String())
		case encoding.TextMarshaler:
			if b, err := x.MarshalText(); err == nil {
				return quoteIfNeeded(string(b))
			}
		}
		return quoteIfNeeded(fmt.Sprintf("%+v", v.Any()))
	default:
		return v.String()
	}
}

// quoteIfNeeded 在字串為空或含空白、引號、等號、控制字元時加上引號，避免 key=value 難以辨識。
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || !unicode.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return s
}

func (h *PrettyHandler) paint(buf []byte, color, s string) []byte {
	if !h.opts.Color {
		return append(buf, s...)
	}
	buf = append(buf, color...)
	buf = append(buf, s...)
	return append(buf, ansiReset...)
}
