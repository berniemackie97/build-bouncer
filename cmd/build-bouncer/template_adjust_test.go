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

func TestGradleOverridePreservesTask(t *testing.T) {
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
			{Name: "build", Run: "./gradlew assemble"},
		},
	}

	applyTemplateOverrides(root, "android", cfg)

	want := "./gradlew"
	if runtime.GOOS == "windows" {
		want = ".\\gradlew.bat"
	}
	if cfg.Checks[0].Run != want+" test" {
		t.Fatalf("unexpected tests command: %q", cfg.Checks[0].Run)
	}
	if cfg.Checks[1].Run != want+" assemble" {
		t.Fatalf("unexpected build command: %q", cfg.Checks[1].Run)
	}
}

func TestNodeTemplateOverridesUsesBun(t *testing.T) {
	root := t.TempDir()
	pkg := `{
  "packageManager": "bun@1.1.0",
  "scripts": {
    "lint": "bun lint",
    "build": "bun build"
  }
}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bun.lockb"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("write bun.lockb: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "lint", Run: "npm run lint"},
			{Name: "tests", Run: "npm run test"},
			{Name: "build", Run: "npm run build"},
		},
	}

	applyTemplateOverrides(root, "node", cfg)

	if len(cfg.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(cfg.Checks))
	}
	if cfg.Checks[0].Run != "bun run lint" || cfg.Checks[1].Run != "bun run build" {
		t.Fatalf("unexpected run commands: %+v", cfg.Checks)
	}
}

func TestPythonTemplateOverridesPoetryRunner(t *testing.T) {
	root := t.TempDir()
	pyproject := `
[tool.poetry]
name = "demo"
`
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(pyproject), 0o644); err != nil {
		t.Fatalf("write pyproject.toml: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "lint", Run: "python -m ruff check ."},
			{Name: "format", Run: "python -m black --check ."},
			{Name: "tests", Run: "python -m pytest"},
		},
	}

	applyTemplateOverrides(root, "python", cfg)

	if cfg.Checks[0].Run != "poetry run ruff check ." {
		t.Fatalf("unexpected lint command: %q", cfg.Checks[0].Run)
	}
	if cfg.Checks[1].Run != "poetry run black --check ." {
		t.Fatalf("unexpected format command: %q", cfg.Checks[1].Run)
	}
	if cfg.Checks[2].Run != "poetry run pytest" {
		t.Fatalf("unexpected tests command: %q", cfg.Checks[2].Run)
	}
}

func TestPythonTemplateOverridesUvRunner(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "uv.lock"), []byte("lock"), 0o644); err != nil {
		t.Fatalf("write uv.lock: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "lint", Run: "python -m ruff check ."},
			{Name: "format", Run: "python -m black --check ."},
			{Name: "tests", Run: "python -m pytest"},
		},
	}

	applyTemplateOverrides(root, "python", cfg)

	if cfg.Checks[0].Run != "uv run ruff check ." {
		t.Fatalf("unexpected lint command: %q", cfg.Checks[0].Run)
	}
	if cfg.Checks[1].Run != "uv run black --check ." {
		t.Fatalf("unexpected format command: %q", cfg.Checks[1].Run)
	}
	if cfg.Checks[2].Run != "uv run pytest" {
		t.Fatalf("unexpected tests command: %q", cfg.Checks[2].Run)
	}
}

func TestRustTemplateOverridesComponents(t *testing.T) {
	root := t.TempDir()
	toolchain := `
[toolchain]
channel = "stable"
components = ["rustfmt"]
`
	if err := os.WriteFile(filepath.Join(root, "rust-toolchain.toml"), []byte(toolchain), 0o644); err != nil {
		t.Fatalf("write rust-toolchain.toml: %v", err)
	}

	cfg := &config.Config{
		Version: 1,
		Checks: []config.Check{
			{Name: "fmt", Run: "cargo fmt --check"},
			{Name: "clippy", Run: "cargo clippy -- -D warnings"},
			{Name: "tests", Run: "cargo test"},
		},
	}

	applyTemplateOverrides(root, "rust", cfg)

	if len(cfg.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(cfg.Checks))
	}
	if cfg.Checks[0].Name == "clippy" || cfg.Checks[1].Name == "clippy" {
		t.Fatalf("expected clippy removed, got %+v", cfg.Checks)
	}
}
