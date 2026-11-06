//go:build windows

package runner

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestInstalledWindowsShellsExecuteChecks(t *testing.T) {
	tests := []struct {
		name string
		shell string
		command string
	}{
		{name: "Command Prompt", shell: "cmd", command: "echo ready"},
		{name: "PowerShell", shell: "pwsh", command: "Write-Output ready"},
		{name: "Git Bash", shell: "bash", command: "printf ready"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shell == "bash" {
				if findGitShell("bash") == "" {
					found, err := exec.LookPath("bash")
					if err != nil || detectShellFlavorPath(found) == pathFlavorWSL {
						t.Skip("Git Bash is not installed")
					}
				}
			} else if _, err := exec.LookPath(tc.shell); err != nil {
				t.Skipf("%s is not installed: %v", tc.shell, err)
			}

			check := config.Check{Name: tc.name, Shell: tc.shell, Run: tc.command}
			report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{check}}, Options{})
			if err != nil || len(report.Failures) != 0 || len(report.Skipped) != 0 || len(report.Canceled) != 0 {
				t.Fatalf("installed shell failed to run check: %+v, %v", report, err)
			}
		})
	}
}

func TestWSLExecutesLinuxScriptWhenDistributionIsInitialized(t *testing.T) {
	launcher, err := exec.LookPath("wsl.exe")
	if err != nil {
		t.Skip("WSL launcher is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, launcher, "--exec", "sh", "-c", "printf ready").CombinedOutput()
	if err != nil {
		t.Skipf("WSL default distribution is not ready: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "ready") {
		t.Fatalf("WSL probe did not run Linux shell: %q", output)
	}

	check := config.Check{Name: "WSL execution", Shell: "wsl.exe", Run: "printf ready"}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{check}}, Options{})
	if err != nil || len(report.Failures) != 0 || len(report.Skipped) != 0 || len(report.Canceled) != 0 {
		t.Fatalf("initialized WSL distribution failed the runner check: %+v, %v", report, err)
	}
}
