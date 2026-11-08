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

func TestWindowsCancelTerminatesStartedDescendant(t *testing.T) {
	root := t.TempDir()
	ready := filepath.Join(root, "ready.txt")
	marker := filepath.Join(root, "survived.txt")
	script := fmt.Sprintf("Set-Content -LiteralPath '%s' -Value ready; Start-Sleep -Seconds 3; Set-Content -LiteralPath '%s' -Value survived", ready, marker)
	commandText := fmt.Sprintf("powershell -NoProfile -NonInteractive -Command \"%s\"", script)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		outcome runOutcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		outcome, err := runOne(ctx, root, root, 0, "started-child", commandText, "cmd", nil, time.Minute, Options{})
		done <- result{outcome, err}
	}()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case finished := <-done:
			t.Fatalf("check exited before child reached ready state: %+v %v", finished.outcome, finished.err)
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("child did not become ready")
		}
		time.Sleep(15 * time.Millisecond)
	}
	cancel()
	select {
	case finished := <-done:
		if finished.err != nil || !finished.outcome.Canceled {
			t.Fatalf("cancellation was not reported: %+v %v", finished.outcome, finished.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled check failed to return promptly")
	}
	time.Sleep(3200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}
