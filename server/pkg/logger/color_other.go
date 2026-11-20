//go:build !windows

package logger

import "io"

func enableVirtualTerminal(io.Writer) bool { return true }
