package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"build-bouncer/internal/config"
)

func TestNodeTemplateOverridesFromScripts(t *testing.T) {
	root := t.TempDir()
	pkg := `{
  "packageManager": "pnpm@9.0.0",
  "scripts": {
    "check": "astro check",
    "build": "astro build"
  }
}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("write pnpm lock: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "lint", Run: "npm run lint"},
			{Name: "tests", Run: "npm run test"},
			{Name: "build", Run: "npm run build"},
		},
	}

	applyTemplateOverrides(root, "astro", cfg)

	if len(cfg.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(cfg.Checks))
	}
	if cfg.Checks[0].Name != "check" || cfg.Checks[1].Name != "build" {
		t.Fatalf("unexpected checks: %+v", cfg.Checks)
	}
	if cfg.Checks[0].Run != "pnpm run check" || cfg.Checks[1].Run != "pnpm run build" {
		t.Fatalf("unexpected run commands: %+v", cfg.Checks)
	}
}

func TestGradleWrapperOverride(t *testing.T) {
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(root, "gradlew.bat"), []byte(""), 0o644); err != nil {
			t.Fatalf("write gradlew.bat: %v", err)
		}
	} else {
		if err := os.WriteFile(filepath.Join(root, "gradlew"), []byte(""), 0o755); err != nil {
			t.Fatalf("write gradlew: %v", err)
		}
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "tests", Run: "./gradlew test"},
			{Name: "build", Run: "./gradlew build"},
		},
	}

	applyTemplateOverrides(root, "gradle", cfg)

	want := "./gradlew"
	if runtime.GOOS == "windows" {
		want = ".\\gradlew.bat"
	}
	if cfg.Checks[0].Run != want+" test" {
		t.Fatalf("unexpected tests command: %q", cfg.Checks[0].Run)
	}
	if cfg.Checks[1].Run != want+" build" {
		t.Fatalf("unexpected build command: %q", cfg.Checks[1].Run)
	}
}

func TestMavenWrapperOverride(t *testing.T) {
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(root, "mvnw.cmd"), []byte(""), 0o644); err != nil {
			t.Fatalf("write mvnw.cmd: %v", err)
		}
	} else {
		if err := os.WriteFile(filepath.Join(root, "mvnw"), []byte(""), 0o755); err != nil {
			t.Fatalf("write mvnw: %v", err)
		}
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "tests", Run: "mvn test"},
			{Name: "build", Run: "mvn -DskipTests package"},
		},
	}

	applyTemplateOverrides(root, "maven", cfg)

	want := "./mvnw"
	if runtime.GOOS == "windows" {
		want = ".\\mvnw.cmd"
	}
	if cfg.Checks[0].Run != want+" test" {
		t.Fatalf("unexpected tests command: %q", cfg.Checks[0].Run)
	}
	if cfg.Checks[1].Run != want+" -DskipTests package" {
		t.Fatalf("unexpected build command: %q", cfg.Checks[1].Run)
	}
}
