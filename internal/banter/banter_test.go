package banter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPickerRespectsLocaleAndSavesState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git dir: %v", err)
	}

	pack := Pack{
		Version:         1,
		MaxHistory:      5,
		DefaultCooldown: 0,
		Entries: []Entry{
			{ID: "intro-1", Type: "intro", Locales: []string{"en"}, Text: "hello"},
		},
	}
	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		t.Fatalf("marshal pack: %v", err)
	}
	packPath := filepath.Join(root, "banter.json")
	if err := os.WriteFile(packPath, data, 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	picker, err := Load(root, Config{File: "banter.json", Locale: "en"})
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}

	got := picker.Pick("intro")
	if strings.TrimSpace(got) != "hello" {
		t.Fatalf("expected banter pick to return hello, got %q", got)
	}

	statePath := resolveStatePath(root)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to exist, got %v", err)
	}
}

func TestPickerSkipsMismatchedLocale(t *testing.T) {
	root := t.TempDir()
	pack := Pack{
		Version:         1,
		DefaultCooldown: 0,
		Entries: []Entry{
			{ID: "fr", Type: "intro", Locales: []string{"fr"}, Text: "bonjour"},
		},
	}
	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		t.Fatalf("marshal pack: %v", err)
	}
	packPath := filepath.Join(root, "banter.json")
	if err := os.WriteFile(packPath, data, 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	picker, err := Load(root, Config{File: "banter.json", Locale: "en"})
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	if got := picker.Pick("intro"); got != "" {
		t.Fatalf("expected empty pick for mismatched locale, got %q", got)
	}
}

func TestPickerConcurrentPicks(t *testing.T) {
	root := t.TempDir()
	pack := Pack{
		Version: 1,
		MaxHistory: 8,
		Entries: []Entry{
			{ID: "loading-1", Type: "loading", Text: "checking"},
		},
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "banter.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	picker, err := Load(root, Config{File: "banter.json", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan string, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- picker.Pick("loading")
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result != "checking" {
			t.Errorf("unexpected pick: %q", result)
		}
	}

	saved, err := os.ReadFile(resolveStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(saved, &state); err != nil {
		t.Fatalf("state must remain valid JSON: %v", err)
	}
	if len(state.Recent["loading"]) != pack.MaxHistory {
		t.Fatalf("expected capped history of %d, got %d", pack.MaxHistory, len(state.Recent["loading"]))
	}
}
