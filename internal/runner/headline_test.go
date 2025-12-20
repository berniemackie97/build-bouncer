package runner

import "testing"

func TestExtractHeadlineGoTest(t *testing.T) {
	out := "--- FAIL: TestWidget (0.00s)\nFAIL\n"
	got := ExtractHeadline("tests", out)
	if got != "Test failed: TestWidget" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineJest(t *testing.T) {
	out := "FAIL src/foo.test.ts\n  ● foo\n"
	got := ExtractHeadline("tests", out)
	if got != "Jest failed: src/foo.test.ts" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlinePytest(t *testing.T) {
	out := "FAILED tests/test_api.py::test_foo - AssertionError\n"
	got := ExtractHeadline("tests", out)
	if got != "Pytest failed: tests/test_api.py::test_foo - AssertionError" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineTypeScript(t *testing.T) {
	out := "src/index.ts(10,5): error TS2304: Cannot find name 'x'.\n"
	got := ExtractHeadline("build", out)
	if got != "src/index.ts:10: Cannot find name 'x'." {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineEslint(t *testing.T) {
	out := "src/app.ts\n  10:5  error  Unexpected any  @typescript-eslint/no-explicit-any\n"
	got := ExtractHeadline("lint", out)
	if got != "src/app.ts:10:5: Unexpected any  @typescript-eslint/no-explicit-any" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineRust(t *testing.T) {
	out := "error[E0432]: unresolved import `foo`\n --> src/lib.rs:1:5\n"
	got := ExtractHeadline("build", out)
	if got != "Rust error: unresolved import `foo`" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineTerraform(t *testing.T) {
	out := "Error: Invalid value\n\n  on main.tf line 1\n"
	got := ExtractHeadline("validate", out)
	if got != "Terraform error: Invalid value" {
		t.Fatalf("unexpected headline: %q", got)
	}
}

func TestExtractHeadlineBlack(t *testing.T) {
	out := "would reformat src/app.py\n"
	got := ExtractHeadline("format", out)
	if got != "Black would reformat: src/app.py" {
		t.Fatalf("unexpected headline: %q", got)
	}
}
