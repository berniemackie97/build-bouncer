package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
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
	case "python":
		updatePythonChecks(root, cfg)
	case "rust":
		updateRustChecks(root, cfg)
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
		return []string{"check", "lint", "typecheck", "test", "build"}
	default:
		return []string{"lint", "typecheck", "test", "check", "build"}
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
		case strings.HasPrefix(pm, "bun"):
			return "bun"
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
	if fileExists(filepath.Join(root, "bun.lockb")) || fileExists(filepath.Join(root, "bun.lock")) {
		return "bun"
	}
	return "npm"
}

func updateGradleChecks(root string, cfg *config.Config) {
	runner := detectGradleRunner(root)
	if runner == "" {
		return
	}
	for i := range cfg.Checks {
		cfg.Checks[i].Run = replaceCommandRunner(cfg.Checks[i].Run, runner, isGradleExecutable)
	}
}

func updateMavenChecks(root string, cfg *config.Config) {
	runner := detectMavenRunner(root)
	if runner == "" {
		return
	}
	for i := range cfg.Checks {
		cfg.Checks[i].Run = replaceCommandRunner(cfg.Checks[i].Run, runner, isMavenExecutable)
	}
}

func detectGradleRunner(root string) string {
	if runtime.GOOS == "windows" {
		if fileExists(filepath.Join(root, "gradlew.cmd")) {
			return ".\\gradlew.cmd"
		}
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
		if fileExists(filepath.Join(root, "mvnw.bat")) {
			return ".\\mvnw.bat"
		}
		if fileExists(filepath.Join(root, "mvnw.cmd")) {
			return ".\\mvnw.cmd"
		}
	}
	if fileExists(filepath.Join(root, "mvnw")) {
		return "./mvnw"
	}
	return "mvn"
}

func updatePythonChecks(root string, cfg *config.Config) {
	runner := detectPythonRunner(root)
	if runner == "" {
		return
	}
	for i := range cfg.Checks {
		switch cfg.Checks[i].Name {
		case "lint":
			cfg.Checks[i].Run = pythonToolCommand(runner, "ruff", "check .")
		case "format":
			cfg.Checks[i].Run = pythonToolCommand(runner, "black", "--check .")
		case "tests":
			cfg.Checks[i].Run = pythonToolCommand(runner, "pytest", "")
		}
	}
}

func detectPythonRunner(root string) string {
	pyproject := readFileString(filepath.Join(root, "pyproject.toml"))
	if hasPyprojectTool(pyproject, "uv") || fileExists(filepath.Join(root, "uv.lock")) {
		return "uv run"
	}
	if hasPyprojectTool(pyproject, "poetry") || fileExists(filepath.Join(root, "poetry.lock")) {
		return "poetry run"
	}
	if hasPyprojectTool(pyproject, "pdm") || fileExists(filepath.Join(root, "pdm.lock")) {
		return "pdm run"
	}
	if fileExists(filepath.Join(root, "Pipfile")) {
		return "pipenv run"
	}
	if hasPyprojectTool(pyproject, "rye") || fileExists(filepath.Join(root, "rye.lock")) {
		return "rye run"
	}
	if hasPyprojectTool(pyproject, "hatch") || fileExists(filepath.Join(root, "hatch.toml")) {
		return "hatch run"
	}
	return ""
}

func hasPyprojectTool(pyproject string, tool string) bool {
	if strings.TrimSpace(pyproject) == "" {
		return false
	}
	needle := "[tool." + strings.ToLower(tool) + "]"
	return strings.Contains(strings.ToLower(pyproject), needle)
}

func pythonToolCommand(runner string, tool string, args string) string {
	cmd := runner + " " + tool
	if strings.TrimSpace(args) != "" {
		cmd += " " + args
	}
	return cmd
}

func updateRustChecks(root string, cfg *config.Config) {
	components, ok := rustToolchainComponents(root)
	if !ok {
		return
	}
	out := cfg.Checks[:0]
	for _, check := range cfg.Checks {
		switch check.Name {
		case "fmt":
			if !components["rustfmt"] {
				continue
			}
		case "clippy":
			if !components["clippy"] {
				continue
			}
		}
		out = append(out, check)
	}
	cfg.Checks = out
}

func rustToolchainComponents(root string) (map[string]bool, bool) {
	path := filepath.Join(root, "rust-toolchain.toml")
	if !fileExists(path) {
		path = filepath.Join(root, "rust-toolchain")
		if !fileExists(path) {
			return nil, false
		}
	}
	content := readFileString(path)
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "components")
	if idx == -1 {
		return nil, false
	}
	segment := content[idx:]
	start := strings.Index(segment, "[")
	if start == -1 {
		return nil, false
	}
	segment = segment[start+1:]
	end := strings.Index(segment, "]")
	if end == -1 {
		return nil, false
	}
	list := segment[:end]
	components := map[string]bool{}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		item = strings.ToLower(item)
		if item != "" {
			components[item] = true
		}
	}
	if len(components) == 0 {
		return nil, false
	}
	return components, true
}

func replaceCommandRunner(command string, runner string, match func(string) bool) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return command
	}
	if !match(commandBase(fields[0])) {
		return command
	}
	fields[0] = runner
	return strings.Join(fields, " ")
}

func commandBase(token string) string {
	trimmed := strings.Trim(token, `"'`)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	return strings.ToLower(path.Base(trimmed))
}

func isGradleExecutable(base string) bool {
	switch base {
	case "gradle", "gradlew", "gradlew.bat", "gradlew.cmd":
		return true
	default:
		return false
	}
}

func isMavenExecutable(base string) bool {
	switch base {
	case "mvn", "mvnw", "mvnw.cmd", "mvnw.bat":
		return true
	default:
		return false
	}
}

func readFileString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
