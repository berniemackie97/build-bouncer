//go:build windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"unsafe"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestWindowsProcessEntersJobBeforeExecution(t *testing.T) {
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 10")
	process, err := startCheckProcess(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		process.terminate()
		_ = command.Wait()
		process.close()
	}()

	handle, err := syscall.OpenProcess(processAssignAccess, false, uint32(command.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)

	var assigned uint32
	isProcessInJob := kernel32.NewProc("IsProcessInJob")
	ok, _, callErr := isProcessInJob.Call(uintptr(handle), uintptr(process.job), uintptr(unsafe.Pointer(&assigned)))
	if ok == 0 {
		t.Fatalf("query process job membership: %v", callErr)
	}
	if assigned == 0 {
		t.Fatal("process resumed without membership in its cleanup job")
	}
}

func TestWindowsConcurrentShortLivedChecks(t *testing.T) {
	const count = 96
	checks := make([]config.Check, count)
	for i := range checks {
		checks[i] = config.Check{Name: fmt.Sprintf("quick-%d", i), Run: "echo ok"}
	}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: checks}, Options{MaxParallel: 8})
	if err != nil {
		t.Fatalf("short-lived checks failed to start: %v", err)
	}
	if len(report.Failures) != 0 || len(report.Canceled) != 0 || len(report.Unavailable) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("unexpected check results: %+v", report)
	}
}
