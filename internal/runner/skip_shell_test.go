package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestShellRequirementWithSpacesUsesExecutableParser(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "shell with spaces", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"\"" + path + "\"", path} {
		if strings.Contains(spec, " ") && spec[0] != '"' {
			continue
		}
		check := config.Check{Name: "test", Run: "echo ok", Shell: spec}
		if missing := MissingTools(check); len(missing) != 0 {
			t.Fatalf("existing executable %q was classified missing: %q", spec, missing)
		}
		if reason := SkipReason(check); reason != "" {
			t.Fatalf("existing executable %q was skipped: %q", spec, reason)
		}
	}
}

func TestMissingShellCannotBeMarkedReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing binary")
	check := config.Check{Name: "test", Shell: "\"" + path + "\"", Run: "echo ok"}
	missing := MissingTools(check)
	if len(missing) != 1 || missing[0] != path {
		t.Fatalf("missing executable was not identified: %q", missing)
	}
	if reason := SkipReason(check); !strings.Contains(reason, path) {
		t.Fatalf("missing executable did not fail readiness: %q", reason)
	}
}
