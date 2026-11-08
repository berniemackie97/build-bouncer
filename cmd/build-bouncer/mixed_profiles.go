package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

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
	if err := validateRootNodeManager(root, pkg); err != nil {
		return nil, nil, err
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

func validateRootNodeManager(root string, pkg packageJSON) error {
	if manager := strings.TrimSpace(pkg.PackageManager); manager != "" {
		switch strings.ToLower(strings.SplitN(manager, "@", 2)[0]) {
		case "npm", "pnpm", "yarn", "bun":
			return nil
		default:
			return fmt.Errorf("unsupported root packageManager %q", manager)
		}
	}
	selected := ""
	for _, lock := range []struct {
		file    string
		manager string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"bun.lock", "bun"},
		{"bun.lockb", "bun"},
		{"package-lock.json", "npm"},
		{"npm-shrinkwrap.json", "npm"},
	} {
		if !regularRootFile(root, lock.file) {
			continue
		}
		if selected != "" && selected != lock.manager {
			return fmt.Errorf("conflicting root package-manager lockfiles; declare packageManager in package.json")
		}
		selected = lock.manager
	}
	return nil
}
