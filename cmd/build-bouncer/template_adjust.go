package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"build-bouncer/internal/config"
)

type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

type templateAdjustResult struct {
	Checks []config.Check
	Inputs map[string]string
	Source string
}

func applyTemplateOverrides(root string, templateID string, checks []config.Check) templateAdjustResult {
	out := append([]config.Check{}, checks...)
	inputs := map[string]string{}
	source := ""

	switch templateID {
	case "node", "react", "vue", "angular", "svelte", "nextjs", "nuxt", "astro":
		if nodeChecks, nodeInputs := nodeChecksFromScripts(root, templateID); len(nodeChecks) > 0 {
			out = nodeChecks
			source = "detector:node-scripts"
			for k, v := range nodeInputs {
				inputs[k] = v
			}
		}
	}

	switch templateID {
	case "gradle", "kotlin", "android":
		if updated, runner := updateGradleChecks(root, out); runner != "" {
			out = updated
			inputs["gradle.runner"] = runner
		}
	case "maven":
		if updated, runner := updateMavenChecks(root, out); runner != "" {
			out = updated
			inputs["maven.runner"] = runner
		}
	case "python":
		if updated, runner := updatePythonChecks(root, out); runner != "" {
			out = updated
			inputs["python.runner"] = runner
		}
	case "rust":
		if updated, components := updateRustChecks(root, out); len(components) > 0 {
			out = updated
			inputs["rust.components"] = strings.Join(components, ",")
		}
	}

	if templateID != "" {
		inputs["template.id"] = templateID
	}

	return templateAdjustResult{Checks: out, Inputs: inputs, Source: source}
}

func nodeChecksFromScripts(root string, templateID string) ([]config.Check, map[string]string) {
	pkg, ok := loadPackageJSON(filepath.Join(root, "package.json"))
	if !ok || len(pkg.Scripts) == 0 {
		return nil, nil
	}

	runner := detectNodeRunner(root, pkg)
	order := nodeScriptOrder(templateID)
	var checks []config.Check
	var scripts []string
	for _, script := range order {
		if !hasScript(pkg, script) {
			continue
		}
		scripts = append(scripts, script)
		name := scriptCheckName(script)
		checks = append(checks, config.Check{
			Name: name,
			Run:  fmt.Sprintf("%s run %s", runner, script),
		})
	}
	if len(checks) == 0 {
		return nil, nil
	}
	return checks, map[string]string{
		"node.runner":  runner,
		"node.scripts": strings.Join(scripts, ","),
	}
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

func updateGradleChecks(root string, checks []config.Check) ([]config.Check, string) {
	runner := detectGradleRunner(root)
	if runner == "" {
		return checks, ""
	}
	out := append([]config.Check{}, checks...)
	for i := range out {
		out[i].Run = replaceCommandRunner(out[i].Run, runner, isGradleExecutable)
	}
	return out, runner
}

func updateMavenChecks(root string, checks []config.Check) ([]config.Check, string) {
	runner := detectMavenRunner(root)
	if runner == "" {
		return checks, ""
	}
	out := append([]config.Check{}, checks...)
	for i := range out {
		out[i].Run = replaceCommandRunner(out[i].Run, runner, isMavenExecutable)
	}
	return out, runner
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

func updatePythonChecks(root string, checks []config.Check) ([]config.Check, string) {
	runner := detectPythonRunner(root)
	if runner == "" {
		return checks, ""
	}
	out := append([]config.Check{}, checks...)
	for i := range out {
		switch out[i].Name {
		case "lint":
			out[i].Run = pythonToolCommand(runner, "ruff", "check .")
		case "format":
			out[i].Run = pythonToolCommand(runner, "black", "--check .")
		case "tests":
			out[i].Run = pythonToolCommand(runner, "pytest", "")
		}
	}
	return out, runner
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

func updateRustChecks(root string, checks []config.Check) ([]config.Check, []string) {
	components, ok := rustToolchainComponents(root)
	if !ok {
		return checks, nil
	}
	out := make([]config.Check, 0, len(checks))
	for _, check := range checks {
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
	var names []string
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	return out, names
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
