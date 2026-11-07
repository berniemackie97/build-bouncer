package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

// rootNodeProfile selects only scripts declared by the root package.
// Workspace membership and package-local scripts are never inferred.
func rootNodeProfile(root string, namespaced bool) ([]config.Check, map[string]string, error) {
	path := filepath.Join(root, "package.json")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("root package.json: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("root package.json must be a regular file")
	}
	if info.Size() > 1<<20 {
		return nil, nil, fmt.Errorf("root package.json exceeds the 1 MiB discovery limit")
	}
	pkg, ok := loadPackageJSON(path)
	if !ok {
		return nil, nil, fmt.Errorf("cannot parse root package.json")
	}
	checks, inputs := nodeChecksForPackage(root, "node", pkg)
	if len(checks) == 0 {
		return nil, nil, fmt.Errorf("root package.json has no supported lint, typecheck, test, check, or build scripts")
	}
	if namespaced {
		for i := range checks {
			checks[i].Name = "node-" + checks[i].Name
		}
	}
	inputs["node.scope"] = "root"
	if regularRootFile(root, "pnpm-workspace.yaml") {
		inputs["node.workspace"] = "pnpm"
	}
	if regularRootFile(root, "turbo.json") || regularRootFile(root, "turbo.jsonc") {
		inputs["node.turbo"] = "present"
	}
	return checks, inputs, nil
}

func regularRootFile(root, name string) bool {
	info, err := os.Lstat(filepath.Join(root, name))
	return err == nil && info.Mode().IsRegular()
}
