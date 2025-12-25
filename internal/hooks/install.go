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
	Force    bool
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

	hookPath := filepath.Join(hooksDir, "pre-push")

	// Check if hook exists and is not ours
	if !opts.Force {
		if b, readErr := os.ReadFile(hookPath); readErr == nil {
			isOurs := strings.Contains(string(b), prePushMarker)
			if !isOurs {
				return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to overwrite)")
			}
		}
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

	hookBody := renderPrePushHook(copied)
	if err := os.WriteFile(hookPath, []byte(hookBody), 0o755); err != nil {
		return err
	}

	_ = os.Chmod(hookPath, 0o755)
	return nil
}

func renderPrePushHook(hasCopiedBinary bool) string {
	body := `#!/bin/sh
# build-bouncer pre-push hook v2
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

# Detect git push flags and pass them to build-bouncer
bb_args="check --hook"

# Check for BUILDBOUNCER_SKIP environment variable
if [ -n "${BUILDBOUNCER_SKIP:-}" ]; then
  bb_args="$bb_args --force-push"
fi

# Check GIT_PUSH_OPTION_COUNT for push options (git 2.10+)
if [ -n "${GIT_PUSH_OPTION_COUNT:-}" ] && [ "${GIT_PUSH_OPTION_COUNT}" -gt 0 ]; then
  i=0
  while [ "$i" -lt "${GIT_PUSH_OPTION_COUNT}" ]; do
    opt_var="GIT_PUSH_OPTION_${i}"
    eval "opt_val=\$${opt_var}"
    case "$opt_val" in
      verbose)
        bb_args="$bb_args --verbose"
        ;;
      force)
        bb_args="$bb_args --force-push"
        ;;
    esac
    i=$((i + 1))
  done
fi

# Fallback: parse command line from ps (less reliable but works for older git)
if ! echo "$bb_args" | grep -q -- "--verbose" && ! echo "$bb_args" | grep -q -- "--force-push"; then
  git_push_cmd="$(ps -o args= -p $PPID 2>/dev/null || true)"
  case "$git_push_cmd" in
    *--verbose*|*-v*)
      bb_args="$bb_args --verbose"
      ;;
  esac
  case "$git_push_cmd" in
    *--force*|*-f*)
      bb_args="$bb_args --force-push"
      ;;
  esac
fi

# For interactive prompts to work in Git Bash on Windows, we need to explicitly use the terminal
if [ -t 0 ]; then
  # stdin is already a terminal
  eval "$bb $bb_args"
else
  # stdin is not a terminal (common in hooks), redirect from tty
  if [ -e /dev/tty ]; then
    eval "$bb $bb_args" < /dev/tty
  else
    eval "$bb $bb_args"
  fi
fi
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
