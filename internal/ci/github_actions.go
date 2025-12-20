package ci

import (
	"fmt"
	"os"
	"path/filepath"
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

	for jobKey, job := range wf.Jobs {
		jobLabel := labelFromJob(jobKey, job.Name)
		jobEnv := normalizeEnv(job.Env)
		jobDir := strings.TrimSpace(job.Defaults.Run.WorkingDirectory)

		for _, step := range job.Steps {
			if strings.TrimSpace(step.Run) == "" {
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
