// Package hooks contains helpers for installing, uninstalling, and inspecting the
// git pre-push hook used by build-bouncer.
package hooks

import (
	"bytes"
	"os"
	"path/filepath"
)

type Status struct {
	RepoRoot     string
	HookPath     string
	Installed    bool
	Ours         bool
	CopiedBinary bool
}

func GetStatus() (Status, error) {
	repoRoot, hooksDir, err := repoHooksDir()
	if err != nil {
		return Status{}, err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")
	st := Status{
		RepoRoot: repoRoot,
		HookPath: hookPath,
	}

	// Hook status
	if b, readErr := os.ReadFile(hookPath); readErr == nil {
		st.Installed = true
		st.Ours = bytes.Contains(b, []byte(prePushMarker))
	} else if !os.IsNotExist(readErr) {
		return st, readErr
	}

	// Copied binary status (we only claim it's present if we can stat it)
	p1, p2 := copiedBinaryPaths(hooksDir)

	if p1 != "" {
		if _, statErr := os.Stat(p1); statErr == nil {
			st.CopiedBinary = true
		} else if !os.IsNotExist(statErr) {
			return st, statErr
		}
	}

	if p2 != "" {
		if _, statErr := os.Stat(p2); statErr == nil {
			st.CopiedBinary = true
		} else if !os.IsNotExist(statErr) {
			return st, statErr
		}
	}

	return st, nil
}
