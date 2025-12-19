package hooks

import (
	"os"
	"path/filepath"
	"strings"
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

	if b, err := os.ReadFile(hookPath); err == nil {
		st.Installed = true
		st.Ours = strings.Contains(string(b), prePushMarker)
	} else if !os.IsNotExist(err) {
		return st, err
	}

	p1, p2 := copiedBinaryPaths(hooksDir)
	if _, err := os.Stat(p1); err == nil {
		st.CopiedBinary = true
	}
	if _, err := os.Stat(p2); err == nil {
		st.CopiedBinary = true
	}

	return st, nil
}
