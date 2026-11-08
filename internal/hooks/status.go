package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Status struct {
	RepoRoot      string
	HookPath      string
	Installed     bool
	Ours          bool
	Executable    bool
	CopiedBinary  bool
}

func GetStatus() (Status, error) {
	repoRoot, hooksDir, err := repoHooksDir()
	if err != nil {
		return Status{}, err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	st := Status{RepoRoot: repoRoot, HookPath: hookPath}
	info, err := os.Lstat(hookPath)
	if err == nil {
		if !info.Mode().IsRegular() {
			return st, fmt.Errorf("pre-push hook is not a regular file: %s", hookPath)
		}
		data, err := os.ReadFile(hookPath)
		if err != nil {
			return st, err
		}
		st.Installed = true
		st.Ours = ownedPrePushHook(data)
		st.Executable = runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
	} else if !os.IsNotExist(err) {
		return st, err
	}

	unixPath, windowsPath := copiedBinaryPaths(hooksDir)
	binaryPath := unixPath
	if runtime.GOOS == "windows" {
		binaryPath = windowsPath
	}
	binary, err := os.Lstat(binaryPath)
	if err == nil {
		st.CopiedBinary = binary.Mode().IsRegular() && (runtime.GOOS == "windows" || binary.Mode().Perm()&0o111 != 0)
	} else if !os.IsNotExist(err) {
		return st, err
	}
	return st, nil
}
