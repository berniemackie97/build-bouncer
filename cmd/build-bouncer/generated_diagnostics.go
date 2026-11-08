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
		case "template:node", "template:react", "template:vue", "template:angular",
			"template:svelte", "template:nextjs", "template:nuxt", "template:astro":
			legacyNode = append(legacyNode, check)
		}
	}

	validateDotNetGenerated(root, cfg.Meta.Inputs["dotnet.solution"], dotnet, add)
	validateNodeGenerated(root, node, add)
	validateNodeGenerated(root, legacyNode, add)
	return findings
}

func validateDotNetGenerated(root, solution string, checks []config.Check, add func(string, string)) {
	if len(checks) == 0 {
		return
	}
	if solution != "" {
		target, err := verifyDotNetSolutionPath(root, solution)
		if err != nil {
			add(checks[0].Name, fmt.Sprintf("dotnet.solution %q: %v", solution, err))
			return
		}
		if strings.Contains(target, " ") {
			target = fmt.Sprintf("%q", target)
		}
		for _, check := range checks {
			var verb string
			switch check.Name {
			case "restore":
				verb = "restore"
			case "format":
				verb = "format"
			case "build":
				verb = "build"
			case "tests":
				verb = "test"
			}
			prefix := "dotnet " + verb + " " + target
			if verb == "" || check.Run != prefix && !strings.HasPrefix(check.Run, prefix+" ") {
				add(check.Name, "generated .NET command does not target the recorded solution; refresh the profile")
			}
		}
		return
	}
	count, err := rootDotNetTargets(root)
	if err != nil {
		add(checks[0].Name, fmt.Sprintf("inspect root .NET targets: %v", err))
		return
	}
	if count != 1 {
		for _, check := range checks {
			if check.Run == "dotnet test -c Release" || check.Run == "dotnet format --verify-no-changes" {
				add(check.Name, "root .NET command requires exactly one root solution or project; refresh with configure --dotnet --solution PATH")
			}
		}
	}
}

func validateNodeGenerated(root string, checks []config.Check, add func(string, string)) {
	if len(checks) == 0 {
		return
	}
	namespaced := false
	for _, check := range checks {
		if strings.HasPrefix(check.Name, "node-") {
			namespaced = true
			break
		}
	}
	expected, _, err := rootNodeProfile(root, namespaced)
	if err != nil {
		add(checks[0].Name, fmt.Sprintf("root Node profile: %v", err))
		return
	}
	known := make(map[string]string, len(expected))
	for _, check := range expected {
		known[check.Name] = check.Run
	}
	for _, check := range checks {
		if !matchesGeneratedNodeCommand(check.Run, known[check.Name]) {
			add(check.Name, "root package script no longer matches this generated check; refresh the profile")
		}
	}
}

func matchesGeneratedNodeCommand(actual, expected string) bool {
	index := strings.Index(expected, " run ")
	if index < 0 {
		return false
	}
	for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
		if actual == manager+expected[index:] {
			return true
		}
	}
	return expected[index:] == " run test" && actual == "npm test"
}

func generatedDirectory(root, relative string) error {
	if relative == "" || relative == "." {
		return nil
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return err
	}
	within, err := filepath.Rel(resolvedRoot, target)
	if err != nil {
		return err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path resolves outside the repository")
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
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
		for _, candidate := range []string{config.DefaultConfigPath(root), config.LegacyConfigPath(root)} {
			if _, err := os.Lstat(candidate); err == nil {
				return fmt.Errorf("configuration path %q is unusable", candidate)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect configuration %q: %w", candidate, err)
			}
		}
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	return generatedAdmission(root, cfg)
}
