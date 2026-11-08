//go:build !windows

package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestTimeoutTerminatesDescendants(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "survived")
	commandText := fmt.Sprintf("(sleep 1; echo survived > %q) & wait", marker)
	start := time.Now()
	outcome, err := runOne(context.Background(), root, root, 0, "descendants", commandText, "sh", nil, 120*time.Millisecond, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.TimedOut {
		t.Fatalf("expected timeout, got %+v", outcome)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived timeout: %v", err)
	}
}

func TestCancelTerminatesDescendants(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "survived")
	commandText := fmt.Sprintf("(sleep 1; echo survived > %q) & wait", marker)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(120*time.Millisecond, cancel)
	start := time.Now()
	outcome, err := runOne(ctx, root, root, 0, "cancel", commandText, "sh", nil, time.Minute, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Canceled {
		t.Fatalf("expected cancellation, got %+v", outcome)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation cleanup took %s", elapsed)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}

func TestInheritedPipeDoesNotHang(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	outcome, err := runOne(context.Background(), root, root, 0, "background", "sleep 8 & exit 0", "sh", nil, time.Minute, Options{})
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("waiting for inherited pipe took %s", elapsed)
	}
	if err != nil || outcome.ExitCode == 0 || outcome.LogPath == "" {
		t.Fatalf("inherited pipe must produce a recorded check failure: outcome=%+v err=%v", outcome, err)
	}
	if outcome.Reason == "" {
		t.Fatal("expected an actionable failure reason")
	}
}

func TestCanceledRunDoesNotReportSuccess(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)
	cfg := &config.Config{Checks: []config.Check{{Name: "slow", Run: "sleep 8"}}}
	start := time.Now()
	report, err := RunAllReport(root, cfg, Options{Context: ctx})
	if err != context.Canceled {
		t.Fatalf("expected cancellation error, got report=%+v err=%v", report, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("canceled suite took %s", elapsed)
	}
}

func TestParallelFailFastTerminatesDescendants(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "survived")
	cfg := &config.Config{Checks: []config.Check{
		{Name: "running", Run: fmt.Sprintf("(sleep 1; echo survived > %q) & wait", marker)},
		{Name: "failure", Run: "sleep 0.3; exit 7"},
	}}
	report, err := RunAllReport(root, cfg, Options{MaxParallel: 2, FailFast: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 1 || report.Failures[0] != "failure" {
		t.Fatalf("unexpected failures: %+v", report)
	}
	if len(report.Canceled) != 1 || report.Canceled[0] != "running" {
		t.Fatalf("unexpected canceled checks: %+v", report)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived fail-fast cancellation: %v", err)
	}
}

func TestSuccessfulParentExitCleansUpBackgroundDescendants(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "survived")
	commandText := fmt.Sprintf("(sleep 1; echo survived > %q) >/dev/null 2>&1 & exit 0", marker)
	outcome, err := runOne(context.Background(), root, root, 0, "completed-parent", commandText, "sh", nil, time.Minute, Options{})
	if err != nil || outcome.ExitCode != 0 {
		t.Fatalf("parent should exit successfully: %+v %v", outcome, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("background child survived successful parent exit: %v", err)
	}
}
