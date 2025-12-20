package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"build-bouncer/internal/config"
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
}

type Step struct {
	Name             string                 `yaml:"name"`
	Run              string                 `yaml:"run"`
	Uses             string                 `yaml:"uses"`
	Env              map[string]interface{} `yaml:"env"`
	WorkingDirectory string                 `yaml:"working-directory"`
	If               string                 `yaml:"if"`
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
		jobLabel := labelFromJob(jobKey, job.Name)
		jobEnv := normalizeEnv(job.Env)
		jobDir := strings.TrimSpace(job.Defaults.Run.WorkingDirectory)

		for _, step := range job.Steps {
			if strings.TrimSpace(step.Run) == "" {
				continue
			}
			if !stepAppliesToOS(step, currentOS) {
				continue
			}

			stepLabel := labelFromStep(step)
			stepEnv := normalizeEnv(step.Env)
			env := mergeEnv(jobEnv, stepEnv)

			cwd := strings.TrimSpace(step.WorkingDirectory)
			if cwd == "" {
				cwd = jobDir
			}

			name := fmt.Sprintf("ci:%s:%s:%s", workflowLabel, jobLabel, stepLabel)
			out = append(out, config.Check{
				Name: name,
				Run:  strings.TrimSpace(step.Run),
				Cwd:  cwd,
				Env:  env,
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
