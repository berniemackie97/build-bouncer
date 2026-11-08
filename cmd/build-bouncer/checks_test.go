package main

import (
	"strings"
	"testing"
	"time"

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

	res, err := mergeChecks(existing, additions)
	if err != nil {
		t.Fatal(err)
	}
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
	merged, err := mergeChecks(existing, additions)
	if err != nil {
		t.Fatal(err)
	}
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

func TestMergeChecksPreservesDistinctPlatforms(t *testing.T) {
	first := config.Check{Name: "ci:test:linux", Run: "go test ./...", Shell: "bash", OS: config.StringList{"linux"}}
	second := config.Check{Name: "ci:test:macos", Run: "go test ./...", Shell: "bash", OS: config.StringList{"macos"}}
	result, err := mergeChecks(nil, []config.Check{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 2 || len(result.Skipped) != 0 {
		t.Fatalf("OS-specific checks were merged together: %+v", result)
	}
}

func TestGeneratedTemplateShellIsPortable(t *testing.T) {
	source := []config.Check{{Name: "go tests", Run: "go test ./..."}}
	generated := stampGeneratedChecks(source, "template:go")
	if len(generated) != 1 || generated[0].Shell != "" || generated[0].ID == "" {
		t.Fatalf("generated template check must have no platform-specific default shell: %+v", generated)
	}
}

func TestCheckContentKeySharesDirectShellParser(t *testing.T) {
	canonical := config.Check{Shell: "bash", Run: "go test ./..."}
	direct := config.Check{Run: "bash -lc \"go test ./...\""}
	if checkContentKey(canonical) != checkContentKey(direct) {
		t.Fatal("matching direct invocation was not deduplicated")
	}
	for _, tc := range []struct {
		configured config.Check
		direct     config.Check
	}{
		{config.Check{Shell: "bash", Run: "go test ./..."}, config.Check{Run: "bash -c \"go test ./...\""}},
		{config.Check{Shell: "sh", Run: "go test ./..."}, config.Check{Run: "sh -lc \"go test ./...\""}},
	} {
		if checkContentKey(tc.configured) == checkContentKey(tc.direct) {
			t.Fatalf("different shell invocation flags were deduplicated: %+v %+v", tc.configured, tc.direct)
		}
	}
	sh := config.Check{Shell: "sh", Run: "go test ./..."}
	if checkContentKey(sh) != checkContentKey(config.Check{Run: "sh -c \"go test ./...\""}) {
		t.Fatal("matching sh invocation was not deduplicated")
	}
	malformed := config.Check{Run: "bash -lc \"go test ./..."}
	if checkContentKey(canonical) == checkContentKey(malformed) {
		t.Fatal("malformed direct invocation was normalized into an executable check")
	}
}

func TestMergeChecksPreservesEnforcementDifferences(t *testing.T) {
	base := config.Check{
		Name: "manual", Run: "go test ./...", Shell: "bash",
		OS: config.StringList{"linux"}, Requires: config.StringList{"go"},
	}
	cases := []struct {
		name   string
		change func(*config.Check)
	}{
		{name: "allow failure", change: func(c *config.Check) { c.AllowFailure = true }},
		{name: "required prerequisites", change: func(c *config.Check) { c.Requires = config.StringList{"go", "git"} }},
		{name: "timeout", change: func(c *config.Check) { c.Timeout = time.Minute }},
		{name: "allow missing tools", change: func(c *config.Check) { c.AllowMissingTools = true }},
		{name: "environment delimiters", change: func(c *config.Check) { c.Env = map[string]string{"A": "x;B=y"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			different := base
			different.Name = "generated"
			tc.change(&different)
			if checkContentKey(base) == checkContentKey(different) {
				t.Fatal("different check semantics share an identity")
			}
			merged, err := mergeChecks([]config.Check{base}, []config.Check{different})
			if err != nil || len(merged.Added) != 1 || len(merged.Skipped) != 0 {
				t.Fatalf("different check was suppressed: %+v %v", merged, err)
			}
		})
	}
	one := base
	one.Env = map[string]string{"A": "x;B=y"}
	two := base
	two.Env = map[string]string{"A": "x", "B": "y"}
	if checkContentKey(one) == checkContentKey(two) {
		t.Fatal("ambiguous environment delimiters collapsed distinct environments")
	}
	original := base
	original.Env = map[string]string{"B": "2", "A": "1"}
	reordered := base
	reordered.Env = map[string]string{"A": "1", "B": "2"}
	reordered.Requires = config.StringList{"go"}
	if checkContentKey(original) != checkContentKey(reordered) {
		t.Fatal("map iteration order changed the content key")
	}
}

func TestMergeChecksRejectsConflictingIDs(t *testing.T) {
	original := config.Check{ID: "same-id", Name: "original", Run: "echo first"}
	changed := config.Check{ID: "same-id", Name: "changed", Run: "echo second"}
	if _, err := mergeChecks([]config.Check{original}, []config.Check{changed}); err == nil || !strings.Contains(err.Error(), "conflicting content") {
		t.Fatalf("an existing generated check ID hid different content: %v", err)
	}
	if _, err := mergeChecks([]config.Check{original, changed}, nil); err == nil {
		t.Fatal("inconsistent existing IDs must fail closed")
	}
}

func TestGeneratedCheckIDTracksEnforcementChanges(t *testing.T) {
	base := config.Check{Name: "tests", Run: "go test ./..."}
	defaultID := stableCheckID("template:go", base)
	if defaultID != "template:go:0058d81af378" {
		t.Fatalf("default generated identity changed, breaking existing configuration: %q", defaultID)
	}
	for _, change := range []func(*config.Check){
		func(c *config.Check) { c.AllowFailure = true },
		func(c *config.Check) { c.Timeout = time.Minute },
	} {
		edited := base
		change(&edited)
		if stableCheckID("template:go", edited) == defaultID {
			t.Fatalf("generated ID accepted modified policy: %+v", edited)
		}
	}
}
