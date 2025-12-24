package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestRunAllReportHonorsEnvAndCwd(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("create work dir: %v", err)
	}

	envKey := "BB_TEST_ENV"
	envVal := "bouncer"

	cmd := ""
	if runtime.GOOS == "windows" {
		cmd = "cd && echo %" + envKey + "% && exit /b 7"
	} else {
		cmd = "pwd; echo $" + envKey + "; exit 7"
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{
				Name: "env-cwd",
				Run:  cmd,
				Cwd:  "work",
				Env: map[string]string{
					envKey: envVal,
				},
			},
		},
	}

	rep, err := RunAllReport(root, cfg, Options{})
	if err != nil {
		t.Fatalf("RunAllReport error: %v", err)
	}

	if len(rep.Failures) != 1 || rep.Failures[0] != "env-cwd" {
		t.Fatalf("expected env-cwd failure, got %+v", rep.Failures)
	}

	tail := rep.FailureTails["env-cwd"]
	if !strings.Contains(strings.ToLower(tail), strings.ToLower(envVal)) {
		t.Fatalf("expected tail to include env value, got %q", tail)
	}
	if !strings.Contains(strings.ToLower(tail), strings.ToLower(workDir)) {
		t.Fatalf("expected tail to include cwd, got %q", tail)
	}

	logPath := rep.LogFiles["env-cwd"]
	if logPath == "" {
		t.Fatal("expected log file path for failed check")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected log file to exist, got %v", err)
	}
}
