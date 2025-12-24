package hooks

import (
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/git"
)

const prePushMarker = "# build-bouncer pre-push hook v1"

func repoHooksDir() (repoRoot string, hooksDir string, err error) {
	root, err := git.FindRepoRoot()
	if err != nil {
		return "", "", err
	}

	return root, filepath.Join(root, ".git", "hooks"), nil
}

func copiedBinaryPaths(hooksDir string) (string, string) {
	base := filepath.Join(hooksDir, "bin", "build-bouncer")
	return base, base + ".exe"
}
