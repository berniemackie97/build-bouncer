package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

const (
	maxDotNetScanEntries = 25000
	maxDotNetScanDepth   = 12
	maxDotNetSolutions   = 32
)

type dotNetWorkspace struct {
	solutions    []string
	overflow     bool
	projectCount int
	allLocked    bool
}

func dotnetProfile(root, selected string) ([]config.Check, string, error) {
	workspace, err := inspectDotNetWorkspace(root)
	if err != nil {
		return nil, "", err
	}
	solution, err := workspace.solution(root, selected)
	if err != nil {
		return nil, "", err
	}
	target := solution
	if strings.Contains(target, " ") {
		target = fmt.Sprintf("%q", target)
	}
	restore := "dotnet restore " + target
	if workspace.projectCount > 0 && workspace.allLocked {
		restore += " --locked-mode"
	}
	checks := []config.Check{
		{Name: "restore", Run: restore, Requires: config.StringList{"dotnet"}},
		{Name: "format", Run: "dotnet format " + target + " --verify-no-changes --no-restore", Requires: config.StringList{"dotnet"}},
		{Name: "build", Run: "dotnet build " + target + " --configuration Release --no-restore", Requires: config.StringList{"dotnet"}},
		{Name: "tests", Run: "dotnet test " + target + " --configuration Release --no-build --no-restore", Requires: config.StringList{"dotnet"}},
	}
	return checks, solution, nil
}

func inspectDotNetWorkspace(root string) (dotNetWorkspace, error) {
	workspace := dotNetWorkspace{allLocked: true}
	scanned := 0
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(relative), "/") + 1
		if entry.IsDir() {
			if ignoredDotNetDirectory(entry.Name()) || depth > maxDotNetScanDepth {
				return filepath.SkipDir
			}
		}
		scanned++
		if scanned > maxDotNetScanEntries {
			return fmt.Errorf(".NET discovery exceeds %d entries; select a smaller workspace", maxDotNetScanEntries)
		}
		if !entry.Type().IsRegular() || depth > maxDotNetScanDepth {
			return nil
		}
		if validSolutionName(entry.Name()) {
			if len(workspace.solutions) < maxDotNetSolutions {
				workspace.solutions = append(workspace.solutions, filepath.ToSlash(relative))
			} else {
				workspace.overflow = true
			}
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".csproj", ".fsproj", ".vbproj":
			workspace.projectCount++
			lock, err := os.Lstat(filepath.Join(filepath.Dir(file), "packages.lock.json"))
			if os.IsNotExist(err) {
				workspace.allLocked = false
				return nil
			}
			if err != nil {
				return err
			}
			if !lock.Mode().IsRegular() {
				workspace.allLocked = false
			}
		}
		return nil
	})
	sort.Strings(workspace.solutions)
	return workspace, err
}

func ignoredDotNetDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".buildbouncer", ".vs", ".idea", ".venv", ".next", ".turbo", ".pnpm",
		"node_modules", "bin", "obj", "dist", "build", "out", "coverage", "target":
		return true
	}
	return false
}

func (workspace dotNetWorkspace) solution(root, selected string) (string, error) {
	if selected != "" {
		return verifyDotNetSolutionPath(root, selected)
	}
	if workspace.overflow {
		return "", fmt.Errorf("more than %d .NET solutions found; select one with --solution PATH", maxDotNetSolutions)
	}
	switch len(workspace.solutions) {
	case 0:
		return "", fmt.Errorf("no .sln or .slnx found in the repository; specify a solution with --solution PATH or use custom checks")
	case 1:
		return workspace.solutions[0], nil
	default:
		return "", fmt.Errorf("multiple .NET solutions found: %s; specify --solution PATH", strings.Join(workspace.solutions, ", "))
	}
}

func verifyDotNetSolutionPath(root, selected string) (string, error) {
	relative := strings.ReplaceAll(selected, "\\", "/")
	parts := strings.Split(relative, "/")
	if strings.HasPrefix(relative, "/") || len(parts) == 0 || !validSolutionName(parts[len(parts)-1]) {
		return "", fmt.Errorf("invalid solution path %q: use a repository-relative .sln or .slnx", selected)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !validDotNetPathPart(part) || ignoredDotNetDirectory(part) {
			return "", fmt.Errorf("invalid solution path %q: use a repository-relative path outside generated directories", selected)
		}
	}
	current := root
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("solution %q: %w", selected, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("solution %q traverses a symlink", selected)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("solution %q has a non-directory path component", selected)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", fmt.Errorf("solution %q is not a regular file", selected)
		}
	}
	return relative, nil
}

func validSolutionName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return (ext == ".sln" || ext == ".slnx") && validDotNetPathPart(name)
}

func validDotNetPathPart(name string) bool {
	if name == "" || name == "." || name == ".." || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == ' ' {
			continue
		}
		return false
	}
	return true
}
