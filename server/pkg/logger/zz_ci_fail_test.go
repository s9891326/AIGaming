package logger

import "testing"

// 暫時性：驗證 CI 在測試失敗時會變紅，驗證後會 revert。
func TestCIShouldFail(t *testing.T) { t.Fatal("intentional failure to verify CI goes red") }
