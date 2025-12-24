// Package hooks contains helpers for installing, uninstalling, and inspecting the
// git pre-push hook used by build-bouncer.
package hooks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type InstallOptions struct {
	CopySelf bool
}

func InstallPrePushHook(opts InstallOptions) error {
	repoRoot, hooksDir, err := repoHooksDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	// Enterprise-safe default: never overwrite someone else's hook.
	// (If the CLI supports --force, it can remove first, then call install.)
	if b, readErr := os.ReadFile(hookPath); readErr == nil {
		if !bytes.Contains(b, []byte(prePushMarker)) {
			return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (remove it or reinstall with --force)")
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
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

	_ = repoRoot
	hookBody := renderPrePushHook(copied)

	// Write atomically (Git for Windows + AV/indexing can make this flaky otherwise).
	tmpPath := hookPath + ".tmp"
	_ = os.Remove(tmpPath)

	if err := os.WriteFile(tmpPath, []byte(hookBody), 0o755); err != nil {
		return err
	}

	_ = os.Remove(hookPath) // needed on Windows; rename-over-existing is not allowed
	if err := os.Rename(tmpPath, hookPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Chmod(hookPath, 0o755)
	return nil
}

func renderPrePushHook(hasCopiedBinary bool) string {
	// Key enterprise behaviors:
	// - Hook remains portable across normal repos and worktrees.
	// - If we copied a binary, prefer the binary located next to the hook (hook_dir/bin).
	// - Otherwise fall back to PATH.
	// - Always include the marker so uninstall/status can reliably identify "ours".
	body := `#!/bin/sh
# ` + prePushMarker + `
set -eu

# Resolve repo root (best effort). If git isn't available for some reason, fall back.
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

# Resolve the directory containing this hook (works for worktrees too).
hook_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

cd "$repo_root" || exit 1

bb=""
`

	if hasCopiedBinary {
		body += `
# Prefer repo-pinned binary (copied during hook install).
if [ -x "$hook_dir/bin/build-bouncer" ]; then
  bb="$hook_dir/bin/build-bouncer"
elif [ -x "$hook_dir/bin/build-bouncer.exe" ]; then
  bb="$hook_dir/bin/build-bouncer.exe"
fi
`
	}

	body += `
# Fall back to PATH lookup.
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

func copyFile(src string, dst string, mode os.FileMode) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		// Only surface close error if nothing else failed.
		if cerr := in.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		// Ensure we don't leak the handle; only return close error if we were otherwise successful.
		if cerr := out.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Optional but “enterprise-safe”: flush file contents before rename.
	// Helps avoid surprising truncation on crashes/AV/filesystems.
	if err := out.Sync(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Windows: rename-over-existing is not allowed
	_ = os.Remove(dst)

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if err := os.Chmod(dst, mode); err != nil {
		return err
	}

	return nil
}
