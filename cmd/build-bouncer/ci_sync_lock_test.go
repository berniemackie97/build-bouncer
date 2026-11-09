package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestCISyncPreservesCommentsOnUnchangedAndChangedImports(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "manual", Run: "echo local"}}}); err != nil {
		t.Fatal(err)
	}
	workflow := writeCIWorkflow(t, root, "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: run\n        run: echo initial\n")
	var stdout, stderr bytes.Buffer
	ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("initial import failed: %d %s", code, stderr.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marked := strings.Replace(string(current), "  - name: manual", "  # user-owned check\n  - name: manual", 1)
	marked = "# user configuration\n" + marked + "# keep this note\n"
	if err := os.WriteFile(path, []byte(marked), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("unchanged import failed: %d %s", code, stderr.String())
	}
	afterNoOp, err := os.ReadFile(path)
	if err != nil || string(afterNoOp) != marked {
		t.Fatalf("unchanged import rewrote configuration: err=%v", err)
	}
	if err := os.WriteFile(workflow, []byte("jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: run\n        run: echo updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("changed import failed: %d %s", code, stderr.String())
	}
	updatedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"# user configuration", "# user-owned check", "# keep this note"} {
		if !strings.Contains(string(updatedBytes), marker) {
			t.Fatalf("reconciliation removed comment %q: %s", marker, updatedBytes)
		}
	}
	updated, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Checks) != 2 || updated.Checks[0].Name != "manual" || updated.Checks[1].Run != "echo updated" {
		t.Fatalf("reconciled checks were not preserved: %+v", updated.Checks)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runCISync(ctx); code != exitOK {
		t.Fatalf("repeat import failed: %d %s", code, stderr.String())
	}
	repeated, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(updatedBytes, repeated) {
		t.Fatalf("repeat import rewrote comments or checks: %v", err)
	}
}

func TestCISyncRefusesConcurrentConfigurationEdit(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "manual", Run: "echo local"}}}); err != nil {
		t.Fatal(err)
	}
	writeCIWorkflow(t, root, "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ci\n")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = config.WithEditLock(path, func() error {
		var stdout, stderr bytes.Buffer
		if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage ||
			!strings.Contains(stderr.String(), "edit lock exists") {
			t.Fatalf("concurrent sync bypassed the lock: exit=%d stderr=%q", code, stderr.String())
		}
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, before) {
			t.Fatalf("blocked sync modified configuration: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("sync after lock release failed: exit=%d error=%q", code, stderr.String())
	}
	updated, err := config.Load(path)
	if err != nil || len(updated.Checks) != 2 {
		t.Fatalf("sync after release lost checks: %+v %v", updated, err)
	}
}

type editDuringApproval struct {
	path string
	done bool
}

func (r *editDuringApproval) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	cfg, err := config.Load(r.path)
	if err != nil {
		return 0, err
	}
	cfg.Checks = append(cfg.Checks, config.Check{Name: "concurrent", Run: "echo added"})
	if err := config.Save(r.path, cfg); err != nil {
		return 0, err
	}
	return copy(p, "y\n"), nil
}

func TestConfigureRejectsStaleApprovalWithoutLosingConcurrentCheck(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "manual", Run: "echo local"}}}); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(&editDuringApproval{path: path})
	var stdout, stderr bytes.Buffer
	if code := runConfigure(configureOptions{protection: "strict"}, cli.Context{Stdout: &stdout, Stderr: &stderr}, reader); code != exitUsage ||
		!strings.Contains(stderr.String(), "configuration changed during editing") {
		t.Fatalf("stale configure approval overwrote changes: exit=%d stderr=%q", code, stderr.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Checks) != 2 || cfg.Checks[1].Name != "concurrent" || cfg.Protection.Level == "strict" {
		t.Fatalf("concurrent change was lost or stale approval applied: %+v", cfg)
	}
}

func TestConfigureSaveRefusesConcurrentEditLock(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: []config.Check{{Name: "manual", Run: "echo local"}}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = config.WithEditLock(path, func() error {
		var stdout, stderr bytes.Buffer
		if code := runConfigure(configureOptions{protection: "strict", yes: true}, cli.Context{Stdout: &stdout, Stderr: &stderr}, bufio.NewReader(strings.NewReader(""))); code != exitUsage ||
			!strings.Contains(stderr.String(), "edit lock exists") {
			t.Fatalf("concurrent configure bypassed lock: exit=%d stderr=%q", code, stderr.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, current) {
		t.Fatalf("concurrent configure changed config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.Base(path)+".edit-lock")); !os.IsNotExist(err) {
		t.Fatalf("edit lock remained after success: %v", err)
	}
}
