package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/shell"
	"gopkg.in/yaml.v3"
)

type Workflow struct {
	Name     string                 `yaml:"name"`
	Env      map[string]interface{} `yaml:"env"`
	Defaults Defaults               `yaml:"defaults"`
	Jobs     map[string]Job         `yaml:"jobs"`
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

	// Apply workspace-relative paths without changing script content.
	for i := range checks {
		checks[i].Cwd = normalizeWorkingDirectory(checks[i].Cwd)
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

	if err := validateWorkflowEnv(wf.Env); err != nil {
		return nil, fmt.Errorf("workflow env: %w", err)
	}
	if err := validateWorkflowShell(wf.Defaults.Run.Shell); err != nil {
		return nil, fmt.Errorf("workflow defaults: %w", err)
	}
	if err := rejectActionsExpression(wf.Defaults.Run.WorkingDirectory); err != nil && !isWorkspaceDirectory(wf.Defaults.Run.WorkingDirectory) {
		return nil, fmt.Errorf("workflow working-directory: %w", err)
	}

	workflowLabel := labelFromWorkflow(path, wf.Name)
	var out []config.Check
	currentOS := currentRunnerOS()

	jobKeys := make([]string, 0, len(wf.Jobs))
	for key := range wf.Jobs {
		jobKeys = append(jobKeys, key)
	}
	sort.Strings(jobKeys)
	workflowEnv := normalizeEnv(wf.Env)
	for _, jobKey := range jobKeys {
		job := wf.Jobs[jobKey]
		if err := validateWorkflowJob(jobKey, job); err != nil {
			return nil, err
		}
		if !jobAppliesToOS(job, currentOS) {
			continue
		}
		jobLabel := labelFromJob(jobKey, job.Name)
		jobEnv := mergeEnv(workflowEnv, normalizeEnv(job.Env))
		jobDir := strings.TrimSpace(job.Defaults.Run.WorkingDirectory)
		if jobDir == "" {
			jobDir = strings.TrimSpace(wf.Defaults.Run.WorkingDirectory)
		}
		defaultShell := strings.TrimSpace(job.Defaults.Run.Shell)
		if defaultShell == "" {
			defaultShell = strings.TrimSpace(wf.Defaults.Run.Shell)
		}

		for _, step := range job.Steps {
			if !stepAppliesToOS(step, currentOS) {
				continue
			}

			if strings.TrimSpace(step.Run) == "" && strings.TrimSpace(step.Uses) == "" {
				continue
			}

			if strings.TrimSpace(step.Run) == "" {
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
			shell := resolveShell(step.Shell, defaultShell, currentOS)
			out = append(out, config.Check{
				Name:   name,
				Run:    step.Run,
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
	applies, _ := conditionMatchesOS(job.If, currentOS)
	return applies
}

func stepAppliesToOS(step Step, currentOS string) bool {
	applies, _ := conditionMatchesOS(step.If, currentOS)
	return applies
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

var reMatrixVar = regexp.MustCompile(`(?i)matrix\.([a-z0-9_-]+)`)

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

func normalizeWorkingDirectory(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	for _, marker := range []string{"${{github.workspace}}", "${{ github.workspace }}"} {
		if cwd == marker {
			return ""
		}
		if strings.HasPrefix(cwd, marker+"/") || strings.HasPrefix(cwd, marker+"\\") {
			return cwd[len(marker)+1:]
		}
	}
	return cwd
}
