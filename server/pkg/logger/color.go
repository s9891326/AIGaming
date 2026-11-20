package logger

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// useColor 判斷是否輸出 ANSI 顏色。
// 遵循 NO_COLOR 慣例（https://no-color.org）：有設定時，除非明確指定 always，否則不上色。
func useColor(mode ColorMode, w io.Writer) bool {
	switch mode {
	case ColorAlways:
		enableVirtualTerminal(w) // 盡力開啟；失敗也照樣輸出顏色碼
		return true
	case ColorNever:
		return false
	}

	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return false
	}
	return enableVirtualTerminal(w)
}
