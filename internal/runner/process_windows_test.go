//go:build windows

package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func windowsNestedCheckScript(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quoted := strings.ReplaceAll(executable, "'", "''")
	return fmt.Sprintf("$child = Start-Process -FilePath '%s' -ArgumentList '-test.run=^TestWindowsGrandchildHelper$' -PassThru -NoNewWindow; $child.WaitForExit()", quoted)
}

func TestWindowsGrandchildHelper(t *testing.T) {
	ready := os.Getenv("BUILD_BOUNCER_WINDOWS_CHILD_READY")
	if ready == "" {
		return
	}
	marker := os.Getenv("BUILD_BOUNCER_WINDOWS_CHILD_MARKER")
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if err := os.WriteFile(marker, []byte("survived"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func windowsChildEnv(root string) (string, string, map[string]string) {
	ready := filepath.Join(root, "ready.txt")
	marker := filepath.Join(root, "survived.txt")
	return ready, marker, map[string]string{
		"BUILD_BOUNCER_WINDOWS_CHILD_READY":  ready,
		"BUILD_BOUNCER_WINDOWS_CHILD_MARKER": marker,
	}
}

func TestWindowsTimeoutTerminatesDescendant(t *testing.T) {
	root := t.TempDir()
	ready, marker, env := windowsChildEnv(root)
	start := time.Now()
	outcome, err := runOne(context.Background(), root, root, 0, "powershell-child",
		windowsNestedCheckScript(t), "powershell", env, 2*time.Second, Options{})
	if err != nil || !outcome.TimedOut {
		t.Fatalf("timeout did not stop check: %+v %v", outcome, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("child was never started before timeout: %v", err)
	}
	time.Sleep(3200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived timeout: %v", err)
	}
}

func TestWindowsCancelTerminatesStartedDescendant(t *testing.T) {
	root := t.TempDir()
	ready, marker, env := windowsChildEnv(root)
	script := windowsNestedCheckScript(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		outcome runOutcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		outcome, err := runOne(ctx, root, root, 0, "started-child",
			script, "powershell", env, time.Minute, Options{})
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

func TestWindowsFailFastTerminatesReadyDescendant(t *testing.T) {
	root := t.TempDir()
	ready, marker, env := windowsChildEnv(root)
	fail := fmt.Sprintf("while (-not (Test-Path -LiteralPath '%s')) { Start-Sleep -Milliseconds 40 }; exit 19",
		strings.ReplaceAll(ready, "'", "''"))
	cfg := &config.Config{Checks: []config.Check{
		{Name: "running", Shell: "powershell", Run: windowsNestedCheckScript(t), Env: env, Timeout: 10 * time.Second},
		{Name: "failure", Shell: "powershell", Run: fail, Timeout: 10 * time.Second},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxParallel: 2, FailFast: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "failure" ||
		len(report.Canceled) != 1 || report.Canceled[0] != "running" {
		t.Fatalf("unexpected parallel fail-fast outcomes: %+v", report)
	}
	time.Sleep(3200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived parallel fail-fast: %v", err)
	}
}
