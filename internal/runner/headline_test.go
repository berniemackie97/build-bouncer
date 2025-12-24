package runner

import (
	"strings"
	"testing"
)

func TestExtractHeadline(t *testing.T) {
	t.Run("go test fail", func(t *testing.T) {
		out := "--- FAIL: TestWidget (0.00s)\nFAIL\n"
		got := ExtractHeadline("tests", out)
		if got != "Test failed: TestWidget" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("go test fail with windows newlines", func(t *testing.T) {
		out := "--- FAIL: TestWidget (0.00s)\r\nFAIL\r\n"
		got := ExtractHeadline("tests", out)
		if got != "Test failed: TestWidget" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("go test timeout", func(t *testing.T) {
		out := "panic: test timed out after 5m0s\n"
		got := ExtractHeadline("tests", out)
		if got != "Go test timeout after 5m0s" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("jest fail", func(t *testing.T) {
		out := "FAIL src/foo.test.ts\n  ● foo\n"
		got := ExtractHeadline("tests", out)
		if got != "Jest failed: src/foo.test.ts" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("pytest fail", func(t *testing.T) {
		out := "FAILED tests/test_api.py::test_foo - AssertionError\n"
		got := ExtractHeadline("tests", out)
		if got != "Pytest failed: tests/test_api.py::test_foo - AssertionError" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("typescript error", func(t *testing.T) {
		out := "src/index.ts(10,5): error TS2304: Cannot find name 'x'.\n"
		got := ExtractHeadline("build", out)
		if got != "src/index.ts:10: Cannot find name 'x'." {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("dotnet build error", func(t *testing.T) {
		out := "src/Foo.cs(12,3): error CS0103: The name 'bar' does not exist in the current context\n"
		got := ExtractHeadline("build", out)
		if got != "src/Foo.cs:12: The name 'bar' does not exist in the current context" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("maven error", func(t *testing.T) {
		out := "[ERROR] src/Main.java:[7,9] cannot find symbol\n"
		got := ExtractHeadline("build", out)
		if got != "src/Main.java:7: cannot find symbol" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("eslint error", func(t *testing.T) {
		out := "src/app.ts\n  10:5  error  Unexpected any  @typescript-eslint/no-explicit-any\n"
		got := ExtractHeadline("lint", out)
		if got != "src/app.ts:10:5: Unexpected any  @typescript-eslint/no-explicit-any" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("rust error", func(t *testing.T) {
		out := "error[E0432]: unresolved import `foo`\n --> src/lib.rs:1:5\n"
		got := ExtractHeadline("build", out)
		if got != "Rust error: unresolved import `foo`" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("terraform error", func(t *testing.T) {
		out := "Error: Invalid value\n\n  on main.tf line 1\n"
		got := ExtractHeadline("validate", out)
		if got != "Terraform error: Invalid value" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("black would reformat", func(t *testing.T) {
		out := "would reformat src/app.py\n"
		got := ExtractHeadline("format", out)
		if got != "Black would reformat: src/app.py" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("first match wins when multiple patterns exist", func(t *testing.T) {
		// If output contains multiple recognizable patterns, we must keep the ordering stable.
		// This starts with a go test failure and also includes an "Error:" line.
		out := "--- FAIL: TestWidget (0.00s)\nFAIL\nError: Invalid value\n"
		got := ExtractHeadline("tests", out)
		if got != "Test failed: TestWidget" {
			t.Fatalf("unexpected headline: %q", got)
		}
	})

	t.Run("truncates long headlines", func(t *testing.T) {
		longReason := strings.Repeat("A", headlineMaxLen*2)
		out := "FAILED tests/test_api.py::test_foo - " + longReason + "\n"
		got := ExtractHeadline("tests", out)

		if len(got) != headlineMaxLen {
			t.Fatalf("expected headline length %d, got %d (%q)", headlineMaxLen, len(got), got)
		}
		if !strings.HasSuffix(got, "...") {
			t.Fatalf("expected truncated headline to end with ..., got %q", got)
		}
		if !strings.HasPrefix(got, "Pytest failed: ") {
			t.Fatalf("expected truncated headline to keep prefix, got %q", got)
		}
	})
}
