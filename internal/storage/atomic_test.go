package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestCreateFileNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config.yaml")
	if err := CreateFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CreateFile(target, []byte("new"), 0o600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected existing-file error, got %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old" {
		t.Fatalf("file overwritten: data=%q err=%v", data, err)
	}
}

func TestWriteFileRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra permissions on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "outside")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "config.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(link, []byte("changed"), 0o600); err == nil {
		t.Fatal("symlink destination must be rejected")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "private" {
		t.Fatalf("symlink target was changed: data=%q err=%v", data, err)
	}
}

func TestConcurrentWritesRemainComplete(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "state.json")
	const count = 20
	var group sync.WaitGroup
	failures := make(chan error, count)
	for i := 0; i < count; i++ {
		group.Add(1)
		go func(value byte) {
			defer group.Done()
			payload := make([]byte, 8192)
			for j := range payload {
				payload[j] = value
			}
			failures <- WriteFile(target, payload, 0o600)
		}(byte('a' + i))
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8192 {
		t.Fatalf("file was partially written: %d bytes", len(data))
	}
	for _, char := range data {
		if char != data[0] {
			t.Fatal("concurrent writers interleaved")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestWriteFilePreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("existing mode changed: %v", info.Mode().Perm())
	}
}

func TestWritePrivateStateTightensExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not enforced on Windows")
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateState(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file permissions = %v", info.Mode().Perm())
	}
	info, err = os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("state directory permissions = %v", info.Mode().Perm())
	}
}

func TestWritePrivateStateRejectsSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra permissions on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "outside")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "state")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateState(filepath.Join(link, "state.json"), []byte("secret")); err == nil {
		t.Fatal("symlinked state directory must be rejected")
	}
	if _, err := os.Stat(filepath.Join(target, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state escaped target directory: %v", err)
	}
}

func TestWriteFileMissingDirectoryLeavesOriginalUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(root, "missing", "config.yaml"), []byte("new"), 0o600); !os.IsNotExist(err) {
		t.Fatalf("expected nonexistent directory error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "old" {
		t.Fatalf("unrelated configuration changed: data=%q err=%v", data, err)
	}
}
