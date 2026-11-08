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

type windowsCheckResult struct {
	outcome runOutcome
	err     error
}

func waitForWindowsChild(t *testing.T, ready string, done <-chan windowsCheckResult) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case finished := <-done:
			t.Fatalf("check exited before child reached ready state: %+v %v", finished.outcome, finished.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not become ready")
		}
		time.Sleep(15 * time.Millisecond)
	}
}

type triggeredDeadlineContext struct {
	context.Context
	done chan struct{}
}

func (c *triggeredDeadlineContext) Done() <-chan struct{} {
	return c.done
}

func (c *triggeredDeadlineContext) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestWindowsPerCheckTimeout(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	outcome, err := runOne(context.Background(), root, root, 0, "powershell-timeout",
		"Start-Sleep -Seconds 10", "powershell", nil, 350*time.Millisecond, Options{})
	if err != nil || !outcome.TimedOut {
		t.Fatalf("per-check timeout was not reported: %+v %v", outcome, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
}

func TestWindowsDeadlineTerminatesReadyDescendant(t *testing.T) {
	root := t.TempDir()
	ready, marker, env := windowsChildEnv(root)
	script := windowsNestedCheckScript(t)
	parent := &triggeredDeadlineContext{Context: context.Background(), done: make(chan struct{})}
	defer func() {
		select {
		case <-parent.done:
		default:
			close(parent.done)
		}
	}()
	done := make(chan windowsCheckResult, 1)
	go func() {
		outcome, err := runOne(parent, root, root, 0, "started-child",
			script, "powershell", env, time.Minute, Options{})
		done <- windowsCheckResult{outcome, err}
	}()
	waitForWindowsChild(t, ready, done)
	close(parent.done)
	select {
	case finished := <-done:
		if finished.err != nil || !finished.outcome.TimedOut {
			t.Fatalf("deadline did not terminate ready child: %+v %v", finished.outcome, finished.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadline check did not terminate promptly")
	}
	time.Sleep(3200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived deadline: %v", err)
	}
}

func TestWindowsCancelTerminatesStartedDescendant(t *testing.T) {
	root := t.TempDir()
	ready, marker, env := windowsChildEnv(root)
	script := windowsNestedCheckScript(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan windowsCheckResult, 1)
	go func() {
		outcome, err := runOne(ctx, root, root, 0, "started-child",
			script, "powershell", env, time.Minute, Options{})
		done <- windowsCheckResult{outcome, err}
	}()
	waitForWindowsChild(t, ready, done)
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
