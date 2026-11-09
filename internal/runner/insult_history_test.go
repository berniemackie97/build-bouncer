package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/storage"
)

func insultHistoryFixture(t *testing.T) (string, config.Insults, Report) {
	t.Helper()
	root := t.TempDir()
	pack := insultPack{
		MaxHistory: 32,
		Templates: []insultTemplate{{
			ID: "failure-1", Categories: []string{"tests"}, Locales: []string{"en"},
			Text: "Fix {checks}: {detail}",
		}},
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pack.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, config.Insults{Mode: "snarky", File: "pack.json", Locale: "en"}, Report{Failures: []string{"tests"}}
}

func TestInsultHistoryConcurrentPicksPreserveEverySelection(t *testing.T) {
	root, cfg, report := insultHistoryFixture(t)
	var group sync.WaitGroup
	errors := make(chan error, 12)
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := PickInsult(root, cfg, report)
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent selection failed: %v", err)
		}
	}
	state, err := loadInsultState(resolveInsultStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Recent) != 12 {
		t.Fatalf("concurrent histories overwritten: %+v", state.Recent)
	}
	for _, selected := range state.Recent {
		if selected != "failure-1" {
			t.Fatalf("unexpected selection %q", selected)
		}
	}
}

func TestInsultHistoryPreservesCorruptionAndReportsFallback(t *testing.T) {
	root, cfg, report := insultHistoryFixture(t)
	path := resolveInsultStatePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("{ incomplete state")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	message, err := PickInsult(root, cfg, report)
	if err == nil || !strings.Contains(err.Error(), "parse failure-message history") ||
		!strings.Contains(message, "Push blocked") {
		t.Fatalf("corrupt history reset without diagnostic: message=%q err=%v", message, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(corrupt, after) {
		t.Fatalf("corrupt history overwritten: %q %v", after, err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("history lock retained after error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	message, err = PickInsult(root, cfg, report)
	if err != nil || !strings.Contains(message, "Fix tests") {
		t.Fatalf("history did not recover after repairing file: %q %v", message, err)
	}
}

func TestInsultHistoryReadFailureReportsFallbackWithoutDestroyingFile(t *testing.T) {
	root, cfg, report := insultHistoryFixture(t)
	path := resolveInsultStatePath(root)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	message, err := PickInsult(root, cfg, report)
	if err == nil || !strings.Contains(err.Error(), "read failure-message history") ||
		!strings.Contains(message, "Push blocked") {
		t.Fatalf("unreadable history was accepted: message=%q err=%v", message, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("unreadable history directory replaced: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := PickInsult(root, cfg, report); err != nil {
		t.Fatalf("history did not recover after resolving storage failure: %v", err)
	}
}

func TestInsultHistoryCrossProcessSerialized(t *testing.T) {
	root, cfg, report := insultHistoryFixture(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestInsultHistoryChild$")
	child.Env = append(os.Environ(), "BUILDBOUNCER_INSULT_CHILD_ROOT="+root)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
		}
	})
	message, err := PickInsult(root, cfg, report)
	if err != nil || !strings.Contains(message, "Fix tests") {
		t.Fatalf("parent failed: %q %v", message, err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("child failed: %v", err)
	}
	state, err := loadInsultState(resolveInsultStatePath(root))
	if err != nil || len(state.Recent) != 2 {
		t.Fatalf("cross-process selection was lost: %+v %v", state, err)
	}
}

func TestInsultHistoryChild(t *testing.T) {
	root := os.Getenv("BUILDBOUNCER_INSULT_CHILD_ROOT")
	if root == "" {
		return
	}
	_, err := PickInsult(root, config.Insults{File: "pack.json", Locale: "en"}, Report{Failures: []string{"tests"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInsultHistoryStaleLockFailsWithDiagnostic(t *testing.T) {
	root, cfg, report := insultHistoryFixture(t)
	path := resolveInsultStatePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	unlock, err := storage.LockStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A held lock may be live or abandoned; never bypass it to replace state.
	message, err := PickInsult(root, cfg, report)
	if err == nil || !strings.Contains(err.Error(), "inspect before removing it") ||
		!strings.Contains(message, "Push blocked") {
		t.Fatalf("stale lock was bypassed: message=%q err=%v", message, err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	message, err = PickInsult(root, cfg, report)
	if err != nil || !strings.Contains(message, "Fix tests") {
		t.Fatalf("history did not recover after lock release: %q %v", message, err)
	}
}
