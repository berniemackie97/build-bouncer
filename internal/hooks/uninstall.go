package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func UninstallPrePushHook(force bool) error {
	_, hooksDir, err := repoHooksDir()
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "pre-push")

	if b, err := os.ReadFile(hookPath); err == nil {
		isOurs := strings.Contains(string(b), prePushMarker)
		if !isOurs && !force {
			return fmt.Errorf("pre-push hook exists but was not installed by build-bouncer (use --force to remove)")
		}

		if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	// Best-effort cleanup of copied binary
	p1, p2 := copiedBinaryPaths(hooksDir)
	_ = os.Remove(p1)
	_ = os.Remove(p2)

	// Remove bin dir if empty
	_ = os.Remove(filepath.Join(hooksDir, "bin"))

	return nil
}
