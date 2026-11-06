//go:build windows

package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsTimeoutTerminatesDescendant(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "survived.txt")
	script := fmt.Sprintf("Start-Sleep -Seconds 2; Set-Content -LiteralPath '%s' -Value survived", marker)
	commandText := fmt.Sprintf("powershell -NoProfile -NonInteractive -Command \"%s\"", script)
	start := time.Now()
	outcome, err := runOne(context.Background(), root, root, 0, "powershell-child", commandText, "cmd", nil, 350*time.Millisecond, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut {
		t.Fatalf("expected timeout, got %+v", outcome)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child survived timeout: %v", err)
	}
}
