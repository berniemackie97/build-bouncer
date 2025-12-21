package ci

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"build-bouncer/internal/config"
	"build-bouncer/internal/shell"
	"gopkg.in/yaml.v3"
)

type Workflow struct {
	Name string         `yaml:"name"`
	Jobs map[string]Job `yaml:"jobs"`
}

type Job struct {
	Name     string                 `yaml:"name"`
	Env      map[string]interface{} `yaml:"env"`
	Defaults Defaults               `yaml:"defaults"`
	Steps    []Step                 `yaml:"steps"`
	RunsOn   RunsOn                 `yaml:"runs-on"`
	If       string                 `yaml:"if"`
	Strategy Strategy               `yaml:"strategy"`
}

type Defaults struct {
	Run RunDefaults `yaml:"run"`
}

type RunDefaults struct {
	WorkingDirectory string `yaml:"working-directory"`
	Shell            string `yaml:"shell"`
}

type Step struct {
	Name             string                 `yaml:"name"`
	Run              string                 `yaml:"run"`
	Uses             string                 `yaml:"uses"`
	With             map[string]interface{} `yaml:"with"`
	Env              map[string]interface{} `yaml:"env"`
	WorkingDirectory string                 `yaml:"working-directory"`
	If               string                 `yaml:"if"`
	Shell            string                 `yaml:"shell"`
}

type Strategy struct {
	Matrix map[string]interface{} `yaml:"matrix"`
}

type RunsOn struct {
	Values []string
}

type toolUsage struct {
	node   bool
	goLang bool
	python bool
	dotnet bool
	java   bool
	ruby   bool
}

func (r *RunsOn) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		if strings.TrimSpace(value.Value) != "" {
			r.Values = []string{value.Value}
		}
	case yaml.SequenceNode:
		var out []string
		for _, n := range value.Content {
			if n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) != "" {
				out = append(out, n.Value)
			}
		}
		r.Values = out
	}
	return nil
}

func ChecksFromGitHubActions(root string) ([]config.Check, error) {
	workflowDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var checks []config.Check
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(workflowDir, name)
		fileChecks, err := checksFromWorkflowFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse workflow %s: %w", path, err)
		}
		checks = append(checks, fileChecks...)
	}

	return checks, nil
}

func checksFromWorkflowFile(path string) ([]config.Check, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	workflowLabel := labelFromWorkflow(path, wf.Name)
	var out []config.Check
	currentOS := currentRunnerOS()

	for jobKey, job := range wf.Jobs {
		if !jobAppliesToOS(job, currentOS) {
			continue
		}
		usage := detectToolsUsed(job.Steps)
		jobLabel := labelFromJob(jobKey, job.Name)
		jobEnv := normalizeEnv(job.Env)
		jobDir := strings.TrimSpace(job.Defaults.Run.WorkingDirectory)

		for _, step := range job.Steps {
			if !stepAppliesToOS(step, currentOS) {
				continue
			}

			if strings.TrimSpace(step.Run) == "" && strings.TrimSpace(step.Uses) == "" {
				continue
			}

			if strings.TrimSpace(step.Run) == "" {
				if check := checkFromUsesStep(workflowLabel, jobLabel, jobEnv, jobDir, job.Defaults, step, usage, currentOS); check != nil {
					out = append(out, *check)
				}
				continue
			}

			stepLabel := labelFromStep(step)
			stepEnv := normalizeEnv(step.Env)
			env := mergeEnv(jobEnv, stepEnv)

			name := fmt.Sprintf("ci:%s:%s:%s", workflowLabel, jobLabel, stepLabel)
			cwd := strings.TrimSpace(step.WorkingDirectory)
			if cwd == "" {
				cwd = jobDir
			}
			shell := resolveShell(step.Shell, job.Defaults.Run.Shell, currentOS)
			out = append(out, config.Check{
				Name:   name,
				Run:    strings.TrimSpace(step.Run),
				Shell:  shell,
				Cwd:    cwd,
				Env:    env,
				OS:     config.StringList{currentOS},
				Source: "ci:" + workflowLabel,
			})
		}
	}

	return out, nil
}

func jobAppliesToOS(job Job, currentOS string) bool {
	if !runsOnMatches(job.RunsOn, job.Strategy.Matrix, currentOS) {
		return false
	}
	if known, allowed := evalIfOS(job.If, currentOS); known && !allowed {
		return false
	}
	return true
}

func stepAppliesToOS(step Step, currentOS string) bool {
	if known, allowed := evalIfOS(step.If, currentOS); known && !allowed {
		return false
	}
	return true
}

func labelFromWorkflow(path string, workflowName string) string {
	if strings.TrimSpace(workflowName) != "" {
		return sanitizeLabel(workflowName)
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return sanitizeLabel(base)
}

func labelFromJob(jobKey string, jobName string) string {
	if strings.TrimSpace(jobName) != "" {
		return sanitizeLabel(jobName)
	}
	return sanitizeLabel(jobKey)
}

func labelFromStep(step Step) string {
	if strings.TrimSpace(step.Name) != "" {
		return sanitizeLabel(step.Name)
	}
	firstLine := strings.TrimSpace(strings.Split(step.Run, "\n")[0])
	if firstLine == "" {
		return "step"
	}
	return sanitizeLabel(firstLine)
}

func labelFromUses(step Step, action string) string {
	if strings.TrimSpace(step.Name) != "" {
		return sanitizeLabel(step.Name)
	}
	short := actionShortName(action)
	cache := strings.ToLower(strings.TrimSpace(stepWithString(step, "cache")))
	if cache != "" {
		short = short + "-" + cache
	}
	return sanitizeLabel(short)
}

func sanitizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeEnv(env map[string]interface{}) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func mergeEnv(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func detectToolsUsed(steps []Step) toolUsage {
	usage := toolUsage{}
	for _, step := range steps {
		if strings.TrimSpace(step.Run) == "" {
			continue
		}
		token := commandToken(step.Run)
		switch token {
		case "node", "npm", "pnpm", "yarn", "bun":
			usage.node = true
		case "go":
			usage.goLang = true
		case "python", "python3", "pip", "pip3", "pipenv", "poetry", "pdm", "uv", "rye", "hatch":
			usage.python = true
		case "dotnet":
			usage.dotnet = true
		case "java", "mvn", "mvnw", "gradle", "gradlew":
			usage.java = true
		case "ruby", "bundle", "bundler", "rake":
			usage.ruby = true
		}
	}
	return usage
}

func commandToken(run string) string {
	fields := strings.Fields(run)
	if len(fields) == 0 {
		return ""
	}
	token := strings.Trim(fields[0], `"'`)
	token = strings.ReplaceAll(token, "\\", "/")
	base := path.Base(token)
	return strings.ToLower(base)
}

func usesActionNeeded(action string, usage toolUsage) bool {
	switch action {
	case "actions/setup-node":
		return usage.node
	case "actions/setup-go":
		return usage.goLang
	case "actions/setup-python":
		return usage.python
	case "actions/setup-dotnet":
		return usage.dotnet
	case "actions/setup-java":
		return usage.java
	case "actions/setup-ruby", "ruby/setup-ruby":
		return usage.ruby
	default:
		return true
	}
}

func checkFromUsesStep(workflowLabel string, jobLabel string, jobEnv map[string]string, jobDir string, defaults Defaults, step Step, usage toolUsage, currentOS string) *config.Check {
	action := normalizeUsesAction(step.Uses)
	if action == "" {
		return nil
	}
	if !usesActionNeeded(action, usage) {
		return nil
	}
	run := usesRunCommand(action, step)
	if strings.TrimSpace(run) == "" {
		return nil
	}

	stepEnv := normalizeEnv(step.Env)
	env := mergeEnv(jobEnv, stepEnv)
	cwd := strings.TrimSpace(step.WorkingDirectory)
	if cwd == "" {
		cwd = jobDir
	}
	shell := resolveShell(step.Shell, defaults.Run.Shell, currentOS)

	name := fmt.Sprintf("ci:%s:%s:%s", workflowLabel, jobLabel, labelFromUses(step, action))
	return &config.Check{
		Name:   name,
		Run:    run,
		Shell:  shell,
		Cwd:    cwd,
		Env:    env,
		OS:     config.StringList{currentOS},
		Source: "ci:" + workflowLabel,
	}
}

func normalizeUsesAction(uses string) string {
	s := strings.TrimSpace(uses)
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	if idx := strings.Index(s, "@"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimPrefix(s, "./")
	if strings.HasPrefix(s, "docker://") {
		return ""
	}
	return s
}

func actionShortName(action string) string {
	parts := strings.Split(strings.Trim(action, "/"), "/")
	if len(parts) == 0 {
		return action
	}
	return parts[len(parts)-1]
}

func usesRunCommand(action string, step Step) string {
	switch action {
	case "actions/checkout":
		return ""
	case "actions/cache", "actions/cache/restore", "actions/cache/save":
		return ""
	case "actions/setup-node":
		return setupNodeCommand(step)
	case "actions/setup-go":
		return "go version"
	case "actions/setup-python":
		return "python --version"
	case "actions/setup-java":
		return "java -version"
	case "actions/setup-dotnet":
		return "dotnet --version"
	case "actions/setup-ruby", "ruby/setup-ruby":
		return "ruby --version"
	default:
		return ""
	}
}

func setupNodeCommand(step Step) string {
	cache := strings.ToLower(strings.TrimSpace(stepWithString(step, "cache")))
	switch cache {
	case "pnpm":
		return "pnpm --version"
	case "yarn":
		return "yarn --version"
	case "npm":
		return "npm --version"
	default:
		return "node --version"
	}
}

func resolveShell(stepShell string, defaultShell string, currentOS string) string {
	candidate := strings.TrimSpace(stepShell)
	if candidate == "" {
		candidate = strings.TrimSpace(defaultShell)
	}
	if normalized := shell.Normalize(candidate); normalized != "" {
		return normalized
	}
	return defaultGitHubShell(currentOS)
}

func defaultGitHubShell(currentOS string) string {
	switch currentOS {
	case "windows":
		return "pwsh"
	case "linux", "macos":
		return "bash"
	default:
		return "bash"
	}
}

func stepWithString(step Step, key string) string {
	if step.With == nil {
		return ""
	}
	val, ok := step.With[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(val))
}

func currentRunnerOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return "linux"
	}
}

func runsOnMatches(runsOn RunsOn, matrix map[string]interface{}, currentOS string) bool {
	if len(runsOn.Values) == 0 {
		return true
	}
	values, hasUnknown := expandRunsOn(runsOn.Values, matrix)
	if len(values) == 0 {
		return hasUnknown
	}

	known := false
	for _, v := range values {
		if os := osFromValue(v); os != "" {
			known = true
			if os == currentOS {
				return true
			}
		}
	}

	if known {
		return false
	}
	return true
}

func expandRunsOn(values []string, matrix map[string]interface{}) ([]string, bool) {
	var out []string
	unknown := false
	for _, v := range values {
		key := matrixKeyFromExpr(v)
		if key == "" {
			out = append(out, v)
			continue
		}
		vals := matrixValues(matrix, key)
		if len(vals) == 0 {
			unknown = true
			continue
		}
		out = append(out, vals...)
	}
	return out, unknown
}

var (
	reIfEq       = regexp.MustCompile(`(?i)\b(?:runner\.os|matrix\.os)\s*==\s*['"]([^'"]+)['"]`)
	reIfNe       = regexp.MustCompile(`(?i)\b(?:runner\.os|matrix\.os)\s*!=\s*['"]([^'"]+)['"]`)
	reIfContains = regexp.MustCompile(`(?i)contains\(\s*(?:runner\.os|matrix\.os)\s*,\s*['"]([^'"]+)['"]\s*\)`)
	reMatrixVar  = regexp.MustCompile(`(?i)matrix\.([a-z0-9_-]+)`)
)

func evalIfOS(expr string, currentOS string) (bool, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, true
	}
	lower := strings.ToLower(expr)
	if !strings.Contains(lower, "runner.os") && !strings.Contains(lower, "matrix.os") {
		return false, true
	}

	var eqMatches []string
	for _, m := range reIfEq.FindAllStringSubmatch(expr, -1) {
		eqMatches = append(eqMatches, m[1])
	}
	for _, m := range reIfContains.FindAllStringSubmatch(expr, -1) {
		eqMatches = append(eqMatches, m[1])
	}

	var neMatches []string
	for _, m := range reIfNe.FindAllStringSubmatch(expr, -1) {
		neMatches = append(neMatches, m[1])
	}

	if len(eqMatches) == 0 && len(neMatches) == 0 {
		return false, true
	}

	if len(eqMatches) > 0 {
		allowed := false
		for _, v := range eqMatches {
			if os := osFromValue(v); os != "" && os == currentOS {
				allowed = true
				break
			}
		}
		if !allowed {
			return true, false
		}
	}

	for _, v := range neMatches {
		if os := osFromValue(v); os != "" && os == currentOS {
			return true, false
		}
	}

	return true, true
}

func matrixKeyFromExpr(value string) string {
	if m := reMatrixVar.FindStringSubmatch(value); len(m) == 2 {
		return m[1]
	}
	return ""
}

func matrixValues(matrix map[string]interface{}, key string) []string {
	if len(matrix) == 0 {
		return nil
	}
	raw, ok := matrix[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string{}, v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

func osFromValue(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(s, "windows"):
		return "windows"
	case strings.Contains(s, "macos") || strings.Contains(s, "osx"):
		return "macos"
	case strings.Contains(s, "ubuntu") || strings.Contains(s, "linux"):
		return "linux"
	default:
		return ""
	}
}
