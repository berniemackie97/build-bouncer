package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

type generatedFinding struct {
	check   string
	message string
}

func generatedFindings(root string, cfg *config.Config) []generatedFinding {
	var findings []generatedFinding
	var dotnet, node, legacyNode []config.Check
	add := func(check, message string) {
		findings = append(findings, generatedFinding{check: check, message: message})
	}

	for _, check := range cfg.Checks {
		if !strings.HasPrefix(check.Source, "template:") && !strings.HasPrefix(check.Source, "detector:") {
			continue
		}
		if check.ID != "" && check.ID != stableCheckID(check.Source, check) {
			add(check.Name, "generated check changed without updating ownership; remove source and id to manage it manually")
		}
		if err := generatedDirectory(root, check.Cwd); err != nil {
			add(check.Name, fmt.Sprintf("working directory %q: %v", check.Cwd, err))
		}
		switch check.Source {
		case "template:dotnet":
			dotnet = append(dotnet, check)
		case "detector:node-scripts":
			node = append(node, check)
		case "template:node":
			legacyNode = append(legacyNode, check)
		}
	}

	if len(dotnet) > 0 {
		if solution := cfg.Meta.Inputs["dotnet.solution"]; solution != "" {
			expected, _, err := dotnetProfile(root, solution)
			if err != nil {
				add(dotnet[0].Name, fmt.Sprintf("dotnet.solution %q: %v", solution, err))
			} else {
				checkGeneratedCommands(dotnet, expected, add)
			}
		} else {
			count, err := rootDotNetTargets(root)
			if err != nil {
				add(dotnet[0].Name, fmt.Sprintf("inspect root .NET targets: %v", err))
			} else if count != 1 {
				for _, check := range dotnet {
					if check.Run == "dotnet test -c Release" || check.Run == "dotnet format --verify-no-changes" {
						add(check.Name, "root .NET command requires exactly one root solution or project; refresh with configure --dotnet --solution PATH")
					}
				}
			}
		}
	}

	if len(node) > 0 {
		namespaced := false
		for _, check := range node {
			if strings.HasPrefix(check.Name, "node-") {
				namespaced = true
				break
			}
		}
		expected, _, err := rootNodeProfile(root, namespaced)
		if err != nil {
			add(node[0].Name, fmt.Sprintf("root Node profile: %v", err))
		} else {
			checkGeneratedCommands(node, expected, add)
		}
	}

	if len(legacyNode) > 0 {
		expected, _, err := rootNodeProfile(root, false)
		if err != nil {
			add(legacyNode[0].Name, fmt.Sprintf("root Node profile: %v", err))
		} else {
			for _, check := range legacyNode {
				matched := false
				for _, valid := range expected {
					if valid.Name != check.Name {
						continue
					}
					matched = check.Run == valid.Run ||
						valid.Run == "npm run test" && check.Run == "npm test"
					break
				}
				if !matched {
					add(check.Name, "root package script or selected package manager no longer matches this generated check; refresh the profile")
				}
			}
		}
	}
	return findings
}

func checkGeneratedCommands(actual, expected []config.Check, add func(string, string)) {
	known := make(map[string]string, len(expected))
	for _, check := range expected {
		known[check.Name] = check.Run
	}
	for _, check := range actual {
		if known[check.Name] != check.Run {
			add(check.Name, "generated command no longer matches its selected target; refresh the profile")
		}
	}
}

func generatedDirectory(root, relative string) error {
	if relative == "" || relative == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink component %q is not allowed", part)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", part)
		}
	}
	return nil
}

func rootDotNetTargets(root string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if validSolutionName(name) {
			count++
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".csproj", ".fsproj", ".vbproj":
			count++
		}
	}
	return count, nil
}

func configRootForPath(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == config.ConfigDirName {
		return filepath.Dir(dir)
	}
	return dir
}

func generatedAdmission(root string, cfg *config.Config) error {
	findings := generatedFindings(root, cfg)
	if len(findings) == 0 {
		return nil
	}
	var messages []string
	for _, finding := range findings {
		messages = append(messages, fmt.Sprintf("%s: %s", finding.check, finding.message))
	}
	return fmt.Errorf("invalid generated checks: %s", strings.Join(messages, "; "))
}

func hookAdmission(root string) error {
	path, exists := config.FindConfigInRoot(root)
	if !exists {
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	return generatedAdmission(root, cfg)
}
