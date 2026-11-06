package main

import (
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestMergeChecksSkipsDuplicates(t *testing.T) {
	existing := []config.Check{
		{Name: "tests", Run: "go test ./...", Cwd: "", Env: map[string]string{"CI": "true"}},
	}
	additions := []config.Check{
		{Name: "ci:tests", Run: "go test ./...", Cwd: "", Env: map[string]string{"CI": "true"}},
		{Name: "ci:lint", Run: "go vet ./...", Cwd: ""},
	}

	res := mergeChecks(existing, additions)
	if len(res.Merged) != 2 {
		t.Fatalf("expected 2 merged checks, got %d", len(res.Merged))
	}
	if len(res.Added) != 1 {
		t.Fatalf("expected 1 added check, got %d", len(res.Added))
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("expected 1 skipped check, got %d", len(res.Skipped))
	}
}

func TestStripManualPlaceholder(t *testing.T) {
	checks := []config.Check{
		{Name: manualPlaceholderName, Run: "echo " + manualPlaceholderSnippet + " 1>&2 && exit 1"},
		{Name: "tests", Run: "go test ./..."},
	}

	stripped := stripManualPlaceholder(checks)
	if len(stripped) != 1 || stripped[0].Name != "tests" {
		t.Fatalf("expected placeholder to be removed, got %+v", stripped)
	}
}

func TestMergeChecksPreservesMissingToolPolicy(t *testing.T) {
	existing := []config.Check{{Name: "required", Run: "go test ./..."}}
	additions := []config.Check{{Name: "optional", Run: "go test ./...", AllowMissingTools: true}}
	merged := mergeChecks(existing, additions)
	if len(merged.Merged) != 2 || len(merged.Skipped) != 0 {
		t.Fatalf("different missing-tool policies must not deduplicate: %+v", merged)
	}
}

func TestGeneratedCheckIDsIncludeMissingToolPolicy(t *testing.T) {
	required := config.Check{Name: "tests", Run: "go test ./..."}
	optional := required
	optional.AllowMissingTools = true
	if stableCheckID("template:go", required) == stableCheckID("template:go", optional) {
		t.Fatal("checks with different missing-tool policies must have distinct IDs")
	}
}
