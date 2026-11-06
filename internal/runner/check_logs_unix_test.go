//go:build !windows

package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareLogDirRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "logs")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := prepareLogDir(link, true); err == nil {
		t.Fatal("log directory symlink must be rejected")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("symlink target mode changed: %v", info.Mode().Perm())
	}
}
