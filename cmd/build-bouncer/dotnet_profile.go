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

func dotnetProfile(root, selected string) ([]config.Check, string, error) {
	solution, err := dotnetSolution(root, selected)
	if err != nil {
		return nil, "", err
	}
	locked, err := allDotNetProjectsLocked(root)
	if err != nil {
		return nil, "", err
	}
	target := solution
	if strings.Contains(target, " ") {
		target = fmt.Sprintf("%q", target)
	}
	restore := "dotnet restore " + target
	if locked {
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

func dotnetSolution(root, selected string) (string, error) {
	if selected != "" {
		if !validSolutionName(selected) {
			return "", fmt.Errorf("invalid solution filename %q: expected a root-level .sln or .slnx file with a portable filename", selected)
		}
		info, err := os.Stat(filepath.Join(root, selected))
		if err != nil {
			return "", fmt.Errorf("solution %q: %w", selected, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("solution %q is not a regular file", selected)
		}
		return selected, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var solutions []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && validSolutionName(entry.Name()) {
			solutions = append(solutions, entry.Name())
		}
	}
	sort.Strings(solutions)
	switch len(solutions) {
	case 0:
		return "", fmt.Errorf("no root-level .sln or .slnx file found; choose a solution explicitly or keep custom checks")
	case 1:
		return solutions[0], nil
	default:
		return "", fmt.Errorf("multiple root-level .NET solutions: %s; specify --solution", strings.Join(solutions, ", "))
	}
}

func validSolutionName(name string) bool {
	if filepath.Base(name) != name || name == "" || name == "." || name == ".." {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".sln" && ext != ".slnx" {
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

func allDotNetProjectsLocked(root string) (bool, error) {
	projectCount := 0
	locked := true
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".buildbouncer", ".vs", "bin", "obj", "node_modules":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".csproj") {
			return nil
		}
		projectCount++
		info, err := os.Stat(filepath.Join(filepath.Dir(path), "packages.lock.json"))
		if os.IsNotExist(err) {
			locked = false
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			locked = false
		}
		return nil
	})
	return projectCount > 0 && locked, err
}
