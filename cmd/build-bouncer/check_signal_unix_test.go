//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestCheckSignalHelper(t *testing.T) {
	if os.Getenv("BUILD_BOUNCER_SIGNAL_TEST") != "1" {
		return
	}
	os.Exit(runCheck([]string{"--ci"}, cli.Context{Stdout: os.Stdout, Stderr: os.Stderr}))
}

func TestCheckCommandStopsOnSIGTERM(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	check := config.Check{
		Name: "slow",
		Run:  "(sleep 1; echo survived > survived.txt) & echo ready > ready.txt; wait",
	}
	cfg := &config.Config{Version: 1, Checks: []config.Check{check}}
	if err := config.Save(filepath.Join(root, ".buildbouncer", "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestCheckSignalHelper$")
	command.Dir = root
	command.Env = append(os.Environ(), "BUILD_BOUNCER_SIGNAL_TEST=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()

	readyPath := filepath.Join(root, "ready.txt")
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("check never started")
		}
		time.Sleep(15 * time.Millisecond)
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	timeout := time.AfterFunc(5*time.Second, func() { _ = command.Process.Kill() })
	err := command.Wait()
	timeout.Stop()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != exitRunFailed {
		t.Fatalf("interrupted check exited with %v, want %d", err, exitRunFailed)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "survived.txt")); !os.IsNotExist(err) {
		t.Fatalf("child survived CLI interruption: %v", err)
	}
}
