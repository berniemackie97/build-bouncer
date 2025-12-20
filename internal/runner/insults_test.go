package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"build-bouncer/internal/config"
)

func TestCategoryFromFailuresPrefersTests(t *testing.T) {
	got := categoryFromFailures([]string{"lint", "tests"})
	if got != "tests" {
		t.Fatalf("expected tests category, got %q", got)
	}
	got = categoryFromFailures([]string{"build-it"})
	if got != "build" {
		t.Fatalf("expected build category, got %q", got)
	}
	got = categoryFromFailures([]string{"ci-step"})
	if got != "ci" {
		t.Fatalf("expected ci category, got %q", got)
	}
}

func TestPickInsultUsesTemplateAndDetail(t *testing.T) {
	root := t.TempDir()
	pack := insultPack{
		Version:         1,
		MaxHistory:      10,
		DefaultCooldown: 0,
		Templates: []insultTemplate{
			{
				ID:         "t1",
				Categories: []string{"tests"},
				Locales:    []string{"en"},
				Text:       "failed {detail}",
			},
		},
	}
	b, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		t.Fatalf("marshal pack: %v", err)
	}
	packPath := filepath.Join(root, "pack.json")
	if err := os.WriteFile(packPath, b, 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	rep := Report{
		Failures:     []string{"tests"},
		FailureTails: map[string]string{"tests": "--- FAIL: ExampleTest\nfatal: boom\n"},
		LogFiles:     map[string]string{},
	}

	msg := PickInsult(root, config.Insults{Mode: "snarky", File: "pack.json", Locale: "en"}, rep)
	if !strings.Contains(msg, "ExampleTest") && !strings.Contains(msg, "boom") {
		t.Fatalf("expected insult to include detail, got %q", msg)
	}

	statePath := resolveInsultStatePath(root)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected insult state to be saved, got: %v", err)
	}
}

func TestExtractDetailPrefersFileLine(t *testing.T) {
	out := "src/app.ts(12,5): error TS1234: nope"
	detail := extractDetailFromOutput("lint", out)
	if detail != "src/app.ts:12:5" {
		t.Fatalf("expected file:line:col detail, got %q", detail)
	}
}

func TestEnsureInsultContextAppendsDetail(t *testing.T) {
	msg := ensureInsultContext("Denied.", "lint", "src/app.ts:12:5")
	if !strings.Contains(msg, "src/app.ts:12:5") {
		t.Fatalf("expected detail appended, got %q", msg)
	}
}
