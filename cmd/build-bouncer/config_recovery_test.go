package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func writeInvalidDirectoryWorkflow(t *testing.T, root string) {
	t.Helper()
	platform := "ubuntu-latest"
	switch runtime.GOOS {
	case "windows":
		platform = "windows-latest"
	case "darwin":
		platform = "macos-latest"
	}
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf("name: Checks\njobs:\n  build:\n    runs-on: %s\n    steps:\n      - name: escaped-dir\n        run: echo ok\n        working-directory: ../../outside\n", platform)
	if err := os.WriteFile(filepath.Join(dir, "checks.yml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInitRollsBackInvalidImportedWorkingDirectory(t *testing.T) {
	root := withTempRepo(t)
	cfgPath := config.DefaultConfigPath(root)
	assets := config.DefaultAssetsPath(root)
	original := map[string]string{
		cfgPath: "user configuration",
		filepath.Join(assets, "insults", "default.json"): "user insults",
		filepath.Join(assets, "banter", "default.json"):  "user banter",
	}
	for path, content := range original {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeInvalidDirectoryWorkflow(t, root)
	var stdout, stderr bytes.Buffer
	code := runInit(true, "go", cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitUsage || !strings.Contains(stderr.String(), "cwd") {
		t.Fatalf("expected unsafe-directory validation failure, exit=%d stderr=%q", code, stderr.String())
	}
	for path, before := range original {
		after, err := os.ReadFile(path)
		if err != nil || string(after) != before {
			t.Fatalf("failed init corrupted %s: %q %v", path, after, err)
		}
	}
}

func TestCISyncPreservesConfigurationWhenImportedDirectoryIsInvalid(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			root := withTempRepo(t)
			path := config.DefaultConfigPath(root)
			if legacy {
				path = config.LegacyConfigPath(root)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			original := []byte("version: 1\nchecks:\n  - name: custom\n    run: echo ok\n")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			writeInvalidDirectoryWorkflow(t, root)
			var stdout, stderr bytes.Buffer
			code := runCISync(cli.Context{Stdout: &stdout, Stderr: &stderr})
			if code != exitUsage || !strings.Contains(stderr.String(), "cwd") {
				t.Fatalf("expected unsafe-directory validation failure, exit=%d stderr=%q", code, stderr.String())
			}
			current, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(current, original) {
				t.Fatalf("failed ci sync corrupted %s: %q %v", path, current, err)
			}
		})
	}
}

func TestValidateRejectsUnknownProtectionLevel(t *testing.T) {
	root := withTempRepo(t)
	path := config.DefaultConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("version: 1\nchecks:\n  - name: test\n    run: echo ok\nprotection:\n  level: strcit\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runValidate("", cli.Context{Stdout: &stdout, Stderr: &stderr})
	if code != exitUsage || !strings.Contains(stderr.String(), "protection.level") {
		t.Fatalf("unknown policy should fail validation, exit=%d stderr=%q", code, stderr.String())
	}
}
