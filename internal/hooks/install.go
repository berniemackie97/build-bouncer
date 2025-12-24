package hooks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/git"
)

type InstallOptions struct {
	CopySelf bool
}

func InstallPrePushHook(opts InstallOptions) error {
	root, err := git.FindRepoRoot()
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	var copied bool
	if opts.CopySelf {
		exe, err := os.Executable()
		if err != nil {
			return err
		}

		binDir := filepath.Join(hooksDir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return err
		}

		dest := filepath.Join(binDir, "build-bouncer")
		if runtime.GOOS == "windows" {
			dest += ".exe"
		}

		if err := copyFile(exe, dest, 0o755); err != nil {
			return fmt.Errorf("copy self into hook bin: %w", err)
		}

		copied = true
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	hookBody := renderPrePushHook(copied)
	if err := os.WriteFile(hookPath, []byte(hookBody), 0o755); err != nil {
		return err
	}

	_ = os.Chmod(hookPath, 0o755)
	return nil
}

func renderPrePushHook(hasCopiedBinary bool) string {
	body := `#!/bin/sh
# build-bouncer pre-push hook v1
set -eu

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root" || exit 1

bb=""
`

	if hasCopiedBinary {
		body += `
if [ -x "$repo_root/.git/hooks/bin/build-bouncer" ]; then
  bb="$repo_root/.git/hooks/bin/build-bouncer"
elif [ -x "$repo_root/.git/hooks/bin/build-bouncer.exe" ]; then
  bb="$repo_root/.git/hooks/bin/build-bouncer.exe"
fi
`
	}

	body += `
if [ -z "$bb" ]; then
  if command -v build-bouncer >/dev/null 2>&1; then
    bb="build-bouncer"
  else
    echo "build-bouncer: not found. Install it or run: build-bouncer hook install" 1>&2
    exit 1
  fi
fi

"$bb" check --hook
`
	return body
}

// copyFile writes dst atomically via a temp file in the same directory, then renames.
// On Windows, AV/indexing can briefly lock new .exe files; we retry rename a few times.
func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp := dst + ".tmp"
	_ = os.Remove(tmp)

	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	copyErr := func() error {
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		// Best-effort flush. Some filesystems/AV combos behave better with an explicit sync.
		_ = out.Sync()
		return nil
	}()

	closeErr := out.Close()

	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}

	// Windows cannot rename-over-existing; also helps if prior attempts left a file behind.
	_ = removeFileWithRetries(dst)

	if err := renameWithRetries(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	_ = os.Chmod(dst, mode)
	return nil
}

func renameWithRetries(from string, to string) error {
	// Non-Windows: rename is usually reliable; keep it simple.
	if runtime.GOOS != "windows" {
		return os.Rename(from, to)
	}

	const attempts = 12
	const delay = 20 * time.Millisecond

	var lastErr error
	for range attempts {
		// Best-effort: if something recreated/left 'to', clear it.
		_ = os.Remove(to)

		lastErr = os.Rename(from, to)
		if lastErr == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return lastErr
}

func removeFileWithRetries(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	// Non-Windows: quick attempt is fine.
	if runtime.GOOS != "windows" {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		return err
	}

	const attempts = 12
	const delay = 15 * time.Millisecond

	var lastErr error
	for range attempts {
		lastErr = os.Remove(path)
		if lastErr == nil || os.IsNotExist(lastErr) {
			return nil
		}
		time.Sleep(delay)
	}
	return lastErr
}
