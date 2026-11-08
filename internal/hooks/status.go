package hooks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"errors"
	"runtime"
)

type Status struct {
	RepoRoot                   string
	HookPath                   string
	Installed                  bool
	Ours                       bool
	Current                    bool
	Executable                 bool
	CopiedBinary               bool
	UsesCopiedBinary           bool
	LegacyGenerated            bool
	CopiedBinaryMatchesRecord  bool
	CopiedBinaryRecordOrphaned bool
}

func GetStatus() (Status, error) {
	repoRoot, hooksDir, err := repoHooksDir()
	if err != nil {
		return Status{}, err
	}
	if err := ensureLifecycleUnlocked(hooksDir); err != nil {
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
		st.Current = generatedPrePushHook(data)
		st.LegacyGenerated = bytes.Equal(data, []byte(renderLegacyPrePushHook(false))) ||
			bytes.Equal(data, []byte(renderLegacyPrePushHook(true)))
		st.UsesCopiedBinary = bytes.Equal(data, []byte(renderPrePushHook(true)))
		st.Executable = runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
	} else if !os.IsNotExist(err) {
		return st, err
	}

	binaryPath := hostCopiedBinaryPath(hooksDir)
	binary, err := os.Lstat(binaryPath)
	if err == nil {
		st.CopiedBinary = binary.Mode().IsRegular() && (runtime.GOOS == "windows" || binary.Mode().Perm()&0o111 != 0)
	} else if !os.IsNotExist(err) {
		return st, err
	}
	if st.UsesCopiedBinary {
		_, owned, err := copiedBinaryOwnership(binaryPath)
		if errors.Is(err, errOrphanedCopiedBinary) {
			st.CopiedBinaryRecordOrphaned = true
		} else if err != nil {
			return st, fmt.Errorf("inspect copied executable ownership: %w", err)
		}
		st.CopiedBinaryMatchesRecord = owned
	}
	if err := ensureLifecycleUnlocked(hooksDir); err != nil {
		return st, err
	}
	return st, nil
}
