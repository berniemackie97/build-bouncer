package banter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	if err := picker.Flush(); err != nil {
		t.Fatal(err)
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
		Version:    1,
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
	if err := picker.Flush(); err != nil {
		t.Fatal(err)
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

func TestSeparatePickersPreserveSharedHistory(t *testing.T) {
	root := t.TempDir()
	pack := Pack{
		Version:    1,
		MaxHistory: 8,
		Entries: []Entry{
			{ID: "intro", Type: "intro", Text: "hello"},
			{ID: "success", Type: "success", Text: "done"},
		},
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "banter.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Pick("intro") != "hello" || second.Pick("success") != "done" {
		t.Fatal("unexpected pick")
	}

	var group sync.WaitGroup
	errors := make(chan error, 2)
	for _, picker := range []*Picker{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- picker.Flush()
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	saved, err := os.ReadFile(resolveStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(saved, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Recent["intro"]) != 1 || state.Recent["intro"][0] != "intro" {
		t.Fatalf("lost intro history: %+v", state.Recent)
	}
	if len(state.Recent["success"]) != 1 || state.Recent["success"][0] != "success" {
		t.Fatalf("lost success history: %+v", state.Recent)
	}
}

func TestPickDefersStatePersistence(t *testing.T) {
	root := t.TempDir()
	pack := Pack{Entries: []Entry{{ID: "intro", Type: "intro", Text: "hello"}}}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "banter.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	picker, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	if picker.Pick("intro") != "hello" {
		t.Fatal("expected pick")
	}
	if _, err := os.Stat(resolveStatePath(root)); !os.IsNotExist(err) {
		t.Fatalf("Pick must not write state: %v", err)
	}
	if err := picker.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resolveStatePath(root)); err != nil {
		t.Fatalf("Flush must persist state: %v", err)
	}
}

func TestPickerFlushAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	data, err := json.Marshal(Pack{Entries: []Entry{
		{ID: "parent", Type: "intro", Text: "hello"},
		{ID: "child", Type: "success", Text: "done"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "banter.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	picker, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	if picker.Pick("intro") != "hello" {
		t.Fatal("parent did not select a phrase")
	}
	lockPath := resolveStatePath(root) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(lockPath) })
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "child-ready")
	finished := filepath.Join(root, "child-finished")
	cmd := exec.Command(executable, "-test.run=^TestPickerFlushChild$")
	cmd.Env = append(os.Environ(), "BUILD_BOUNCER_BANTER_CHILD="+root)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not reach banter state flush")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(finished); !os.IsNotExist(err) {
		t.Fatalf("child ignored cross-process state lock: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	if err := picker.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child state flush failed: %v", err)
	}
	state, err := loadState(resolveStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Recent["intro"]) != 1 || state.Recent["intro"][0] != "parent" ||
		len(state.Recent["success"]) != 1 || state.Recent["success"][0] != "child" {
		t.Fatalf("lost updates from parallel processes: %+v", state.Recent)
	}
}

func TestPickerFlushChild(t *testing.T) {
	root := os.Getenv("BUILD_BOUNCER_BANTER_CHILD")
	if root == "" {
		return
	}
	picker, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	if picker.Pick("success") != "done" {
		t.Fatal("child selection failed")
	}
	if err := os.WriteFile(filepath.Join(root, "child-ready"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := picker.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child-finished"), []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPickerPreservesInvalidPersistedState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "banter.json"), []byte(`{"entries":[{"id":"intro","type":"intro","text":"hello"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := resolveStatePath(root)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	invalid := []byte("{incomplete")
	if err := os.WriteFile(statePath, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, Config{File: "banter.json"}); err == nil || !strings.Contains(err.Error(), "parse banter state") {
		t.Fatalf("corrupted state silently reset: %v", err)
	}
	current, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(current, invalid) {
		t.Fatalf("corrupted state overwritten: %v", err)
	}
}

func TestPickerFlushRetainsPendingAfterStateFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "banter.json"), []byte(`{"entries":[{"id":"intro","type":"intro","text":"hello"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	picker, err := Load(root, Config{File: "banter.json"})
	if err != nil {
		t.Fatal(err)
	}
	if picker.Pick("intro") != "hello" {
		t.Fatal("missing pending selection")
	}
	statePath := resolveStatePath(root)
	if err := os.MkdirAll(statePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := picker.Flush(); err == nil {
		t.Fatal("failed state write must be reported")
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if err := picker.Flush(); err != nil {
		t.Fatalf("pending state lost after retry: %v", err)
	}
	saved, err := loadState(statePath)
	if err != nil || len(saved.Recent["intro"]) != 1 {
		t.Fatalf("retry did not preserve selection: %+v %v", saved, err)
	}
}
