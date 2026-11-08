package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func ciSyncFixture(t *testing.T, checks []config.Check, workflow *string) string {
	t.Helper()
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := config.Save(path, &config.Config{Checks: checks}); err != nil {
		t.Fatal(err)
	}
	if workflow != nil {
		dir := filepath.Join(root, ".github", "workflows")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if *workflow != "" {
			if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(*workflow), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return path
}

func TestCISyncReconcilesStaleChecksWithNoRunnableWorkflows(t *testing.T) {
	empty := ""
	setupOnly := "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	for _, tc := range []struct {
		name     string
		workflow *string
	}{
		{name: "workflow directory removed"},
		{name: "empty workflow directory", workflow: &empty},
		{name: "workflow has only setup steps", workflow: &setupOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owned := stampGeneratedChecks([]config.Check{{
				Name: "ci:obsolete:linux:test", Source: "ci:obsolete", Run: "echo obsolete",
			}}, "ci")
			checks := []config.Check{
				{Name: "custom", Run: "echo custom"},
				{Name: "ci:manual", Run: "echo user-authored"},
				{Name: manualPlaceholderName, Run: "echo " + manualPlaceholderSnippet},
				owned[0],
				{Name: "ci:legacy", Source: "ci:legacy", Run: "echo old"},
			}
			path := ciSyncFixture(t, checks, tc.workflow)
			var stdout, stderr bytes.Buffer
			ctx := cli.Context{Stdout: &stdout, Stderr: &stderr}
			if code := runCISync(ctx); code != exitOK {
				t.Fatalf("sync failed: %d %s", code, stderr.String())
			}
			updated, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"custom", "ci:manual", manualPlaceholderName}
			if len(updated.Checks) != len(want) {
				t.Fatalf("stale imported checks retained: %+v", updated.Checks)
			}
			for i, check := range updated.Checks {
				if check.Name != want[i] || check.Source != "" {
					t.Fatalf("custom check lost or changed: %+v", updated.Checks)
				}
			}
			if !strings.Contains(stdout.String(), "Removed 2 stale CI checks") ||
				!strings.Contains(stdout.String(), "No CI checks found") {
				t.Fatalf("missing reconciliation summary: %q", stdout.String())
			}
			afterFirst, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stdout.Reset()
			stderr.Reset()
			if code := runCISync(ctx); code != exitOK {
				t.Fatalf("repeat sync failed: %d %s", code, stderr.String())
			}
			afterSecond, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(afterFirst, afterSecond) {
				t.Fatalf("repeat sync was not byte-idempotent: %v", err)
			}
		})
	}
}

func TestCISyncRefusesToRemoveLastConfiguredCheck(t *testing.T) {
	owned := stampGeneratedChecks([]config.Check{{
		Name: "ci:obsolete:linux:test", Source: "ci:obsolete", Run: "echo obsolete",
	}}, "ci")
	path := ciSyncFixture(t, owned, nil)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage ||
		!strings.Contains(stderr.String(), "would leave no checks configured") {
		t.Fatalf("unsafe empty configuration accepted: exit=%d error=%q", code, stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("refused sync modified existing configuration: %v", err)
	}
}

func TestCISyncChecksOwnershipWithoutRunnableWorkflows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*config.Check)
	}{
		{name: "failure policy", change: func(c *config.Check) { c.AllowFailure = true }},
		{name: "timeout", change: func(c *config.Check) { c.Timeout = time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owned := stampGeneratedChecks([]config.Check{{
				Name: "ci:old:linux:test", Source: "ci:old", Run: "echo old",
			}}, "ci")
			tc.change(&owned[0])
			path := ciSyncFixture(t, append([]config.Check{{Name: "custom", Run: "echo keep"}}, owned[0]), nil)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitUsage ||
				!strings.Contains(stderr.String(), "has been modified") {
				t.Fatalf("edited owned check was removed: exit=%d error=%q", code, stderr.String())
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected sync rewrote configuration: %v", err)
			}
		})
	}
}

func TestCISyncNoOpPreservesManualConfigBytes(t *testing.T) {
	empty := ""
	path := ciSyncFixture(t, []config.Check{
		{Name: "ci:manual", Run: "echo manual"},
		{Name: manualPlaceholderName, Run: "echo " + manualPlaceholderSnippet},
	}, &empty)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	before = append(before, []byte("# user-written comment\n")...)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr}); code != exitOK {
		t.Fatalf("no-op sync failed: exit=%d error=%q", code, stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("no-op sync changed manual config bytes: %v", err)
	}
}
