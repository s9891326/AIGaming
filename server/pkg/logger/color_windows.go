//go:build windows

package logger

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// enableVirtualTerminal 讓 Windows 主控台解讀 ANSI 顏色碼。
// 新版 Windows Terminal / VS Code 終端機通常已開啟，但舊的 conhost 需要程式自己打開。
func enableVirtualTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return true
	}
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		// 不是主控台（例如被導向到管線），保留 ANSI 碼交給接收端處理。
		return true
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
