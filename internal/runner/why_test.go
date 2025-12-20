package runner

import "testing"

func TestExtractWhyGoTestTimeout(t *testing.T) {
	out := "panic: test timed out after 10m0s\n"
	got := ExtractWhy("tests", out)
	if got != "Go test timeout after 10m0s" {
		t.Fatalf("unexpected why: %q", got)
	}
}

func TestExtractWhyRuff(t *testing.T) {
	out := "src/app.py:12:4: F401 unused import\n"
	got := ExtractWhy("lint", out)
	if got != "Ruff F401: src/app.py:12:4: unused import" {
		t.Fatalf("unexpected why: %q", got)
	}
}

func TestExtractWhyMaven(t *testing.T) {
	out := "[ERROR] /path/File.java:[12,8] cannot find symbol\n"
	got := ExtractWhy("build", out)
	if got != "Maven error: /path/File.java:12:8: cannot find symbol" {
		t.Fatalf("unexpected why: %q", got)
	}
}

func TestExtractWhyRustLocation(t *testing.T) {
	out := "error[E0432]: unresolved import `foo`\n --> src/lib.rs:1:5\n"
	got := ExtractWhy("build", out)
	if got != "Rust error: unresolved import `foo` (src/lib.rs:1:5)" {
		t.Fatalf("unexpected why: %q", got)
	}
}
