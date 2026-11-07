package hooks

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/storage"
)

type InstallOptions struct {
	CopySelf bool
	Force    bool
}

func InstallPrePushHook(opts InstallOptions) error {
	_, hooksDir, err := repoHooksDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	previous, mode, existed, err := existingHook(hookPath, opts.Force)
	if err != nil {
		return err
	}

	var stagedBinary, binaryPath string
	if opts.CopySelf {
		binDir := filepath.Join(hooksDir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return err
		}
		defer func() { _ = removeDirIfEmpty(binDir) }()
		info, err := os.Lstat(binDir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("copied executable directory is not a directory: %s", binDir)
		}
		binaryPath = filepath.Join(binDir, "build-bouncer")
		if runtime.GOOS == "windows" {
			binaryPath += ".exe"
		}
		stagedBinary, err = stageExecutable(binDir)
		if err != nil {
			return fmt.Errorf("stage copied binary: %w", err)
		}
		defer os.Remove(stagedBinary)
	}

	body := []byte(renderPrePushHook(opts.CopySelf))
	if existed {
		err = storage.WriteFile(hookPath, body, 0o755)
	} else {
		err = storage.CreateFile(hookPath, body, 0o755)
	}
	if err != nil {
		return fmt.Errorf("install pre-push hook: %w", err)
	}
	if err := os.Chmod(hookPath, 0o755); err != nil {
		return errors.Join(err, restoreHook(hookPath, previous, mode, existed))
	}
	if stagedBinary != "" {
		if err := storage.ReplaceStagedFile(stagedBinary, binaryPath); err != nil {
			return errors.Join(fmt.Errorf("install copied binary: %w", err), restoreHook(hookPath, previous, mode, existed))
		}
	}
	return nil
}

func existingHook(path string, force bool) ([]byte, os.FileMode, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("pre-push hook is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	if !strings.Contains(string(data), prePushMarker) && !force {
		return nil, 0, false, fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to overwrite)")
	}
	return data, info.Mode().Perm(), true, nil
}

func restoreHook(path string, previous []byte, mode os.FileMode, existed bool) error {
	if !existed {
		return removeFileWithRetries(path)
	}
	if err := storage.WriteFile(path, previous, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func stageExecutable(dir string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	in, err := os.Open(executable)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, ".build-bouncer-*.tmp")
	if err != nil {
		return "", err
	}
	defer func() {
		if out != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()
	if err := out.Chmod(0o755); err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	path := out.Name()
	if err := out.Close(); err != nil {
		return "", err
	}
	out = nil
	return path, nil
}

func renderPrePushHook(hasCopiedBinary bool) string {
	body := `#!/bin/sh
# build-bouncer pre-push hook v3
set -eu

hook_dir="$(CDPATH= cd -P "$(dirname "$0")" && pwd)"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root" || exit 1

bb=""
`

	if hasCopiedBinary {
		body += `
case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*)
    if [ -x "$hook_dir/bin/build-bouncer.exe" ]; then
      bb="$hook_dir/bin/build-bouncer.exe"
    fi
    ;;
  *)
    if [ -x "$hook_dir/bin/build-bouncer" ]; then
      bb="$hook_dir/bin/build-bouncer"
    fi
    ;;
esac
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

if [ -t 0 ]; then
  "$bb" check --hook
elif [ -e /dev/tty ]; then
  "$bb" check --hook < /dev/tty
else
  "$bb" check --hook
fi
`
	return body
}
