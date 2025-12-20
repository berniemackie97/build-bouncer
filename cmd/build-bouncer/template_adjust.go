package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"build-bouncer/internal/config"
)

type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

func applyTemplateOverrides(root string, templateID string, cfg *config.Config) {
	switch templateID {
	case "node", "react", "vue", "angular", "svelte", "nextjs", "nuxt", "astro":
		if checks := nodeChecksFromScripts(root, templateID); len(checks) > 0 {
			cfg.Checks = checks
		}
	}

	switch templateID {
	case "gradle", "kotlin", "android":
		updateGradleChecks(root, cfg)
	case "maven":
		updateMavenChecks(root, cfg)
	}
}

func nodeChecksFromScripts(root string, templateID string) []config.Check {
	pkg, ok := loadPackageJSON(filepath.Join(root, "package.json"))
	if !ok || len(pkg.Scripts) == 0 {
		return nil
	}

	runner := detectNodeRunner(root, pkg)
	order := nodeScriptOrder(templateID)
	var checks []config.Check
	for _, script := range order {
		if !hasScript(pkg, script) {
			continue
		}
		name := scriptCheckName(script)
		checks = append(checks, config.Check{
			Name: name,
			Run:  fmt.Sprintf("%s run %s", runner, script),
		})
	}
	return checks
}

func nodeScriptOrder(templateID string) []string {
	switch templateID {
	case "astro":
		return []string{"check", "lint", "test", "build"}
	default:
		return []string{"lint", "test", "check", "build"}
	}
}

func scriptCheckName(script string) string {
	switch script {
	case "test":
		return "tests"
	default:
		return script
	}
}

func hasScript(pkg packageJSON, script string) bool {
	if pkg.Scripts == nil {
		return false
	}
	cmd, ok := pkg.Scripts[script]
	return ok && strings.TrimSpace(cmd) != ""
}

func loadPackageJSON(path string) (packageJSON, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return packageJSON{}, false
	}
	var pkg packageJSON
	if err := json.Unmarshal(b, &pkg); err != nil {
		return packageJSON{}, false
	}
	return pkg, true
}

func detectNodeRunner(root string, pkg packageJSON) string {
	if pm := strings.TrimSpace(pkg.PackageManager); pm != "" {
		pm = strings.ToLower(pm)
		switch {
		case strings.HasPrefix(pm, "pnpm"):
			return "pnpm"
		case strings.HasPrefix(pm, "yarn"):
			return "yarn"
		case strings.HasPrefix(pm, "npm"):
			return "npm"
		}
	}

	if fileExists(filepath.Join(root, "pnpm-lock.yaml")) {
		return "pnpm"
	}
	if fileExists(filepath.Join(root, "yarn.lock")) {
		return "yarn"
	}
	return "npm"
}

func updateGradleChecks(root string, cfg *config.Config) {
	runner := detectGradleRunner(root)
	if runner == "" {
		return
	}
	for i := range cfg.Checks {
		switch cfg.Checks[i].Name {
		case "tests":
			cfg.Checks[i].Run = runner + " test"
		case "build":
			cfg.Checks[i].Run = runner + " build"
		}
	}
}

func updateMavenChecks(root string, cfg *config.Config) {
	runner := detectMavenRunner(root)
	if runner == "" {
		return
	}
	for i := range cfg.Checks {
		switch cfg.Checks[i].Name {
		case "tests":
			cfg.Checks[i].Run = runner + " test"
		case "build":
			cfg.Checks[i].Run = runner + " -DskipTests package"
		}
	}
}

func detectGradleRunner(root string) string {
	if runtime.GOOS == "windows" {
		if fileExists(filepath.Join(root, "gradlew.bat")) {
			return ".\\gradlew.bat"
		}
	}
	if fileExists(filepath.Join(root, "gradlew")) {
		return "./gradlew"
	}
	return "gradle"
}

func detectMavenRunner(root string) string {
	if runtime.GOOS == "windows" {
		if fileExists(filepath.Join(root, "mvnw.cmd")) {
			return ".\\mvnw.cmd"
		}
	}
	if fileExists(filepath.Join(root, "mvnw")) {
		return "./mvnw"
	}
	return "mvn"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
