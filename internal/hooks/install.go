package hooks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"build-bouncer/internal/git"
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

	// Prefer repo-pinned binary first when present.
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

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}

	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	_ = os.Remove(dst)

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	_ = os.Chmod(dst, mode)
	return nil
}
