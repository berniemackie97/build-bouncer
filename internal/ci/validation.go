package ci

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/shell"
)

var simpleOSCondition = regexp.MustCompile(`(?i)^runner\.os\s*(==|!=)\s*['"]([^'"]+)['"]$`)

func validateWorkflowWorkingDirectory(value string) error {
	if err := rejectActionsExpression(value); err != nil && !isWorkspaceDirectory(value) {
		return err
	}
	return nil
}

func validateWorkflowJob(name string, job Job) error {
	if job.Needs != nil || job.Container != nil || job.Services != nil || job.Uses != "" || job.TimeoutMinutes != 0 {
		return fmt.Errorf("job %q: dependencies, containers, services, reusable workflows and job timeouts are not supported by CI import", name)
	}
	if len(job.RunsOn.Values) == 0 {
		return fmt.Errorf("job %q: runs-on is required to determine the target platform", name)
	}
	if len(job.RunsOn.Values) > 1 {
		return fmt.Errorf("job %q: multi-label runs-on cannot be represented by a local check", name)
	}
	if len(job.Strategy.Matrix) > 0 && !isMatrixOSReference(job.RunsOn.Values[0]) {
		return fmt.Errorf("job %q: matrix.os must determine runs-on to generate portable checks", name)
	}
	for key := range job.Strategy.Matrix {
		if key != "os" {
			return fmt.Errorf("job %q: unsupported matrix dimension %q", name, key)
		}
	}
	for _, runTarget := range job.RunsOn.Values {
		target := strings.TrimSpace(runTarget)
		if isMatrixOSReference(target) {
			values := matrixValues(job.Strategy.Matrix, "os")
			if len(values) == 0 {
				return fmt.Errorf("job %q: matrix.os has no supported values", name)
			}
			seenOS := make(map[string]bool, len(values))
			for _, value := range values {
				if !isSupportedRunner(value) {
					return fmt.Errorf("job %q: unsupported matrix runner %q", name, value)
				}
				osName := osFromValue(value)
				if seenOS[osName] {
					return fmt.Errorf("job %q: multiple runner images for %s cannot be represented by one local check", name, osName)
				}
				seenOS[osName] = true
			}
		} else if !isSupportedRunner(target) {
			return fmt.Errorf("job %q: unsupported runs-on %q", name, target)
		}
	}
	if _, err := conditionMatchesOS(job.If, "linux"); err != nil {
		return fmt.Errorf("job %q: %w", name, err)
	}
	if err := validateWorkflowEnv(job.Env); err != nil {
		return fmt.Errorf("job %q: %w", name, err)
	}
	if err := validateWorkflowWorkingDirectory(job.Defaults.Run.WorkingDirectory); err != nil {
		return fmt.Errorf("job %q working-directory: %w", name, err)
	}
	if err := validateWorkflowShell(job.Defaults.Run.Shell); err != nil {
		return fmt.Errorf("job %q: %w", name, err)
	}
	for i, step := range job.Steps {
		if step.TimeoutMinutes != 0 || step.ContinueOnError != nil && *step.ContinueOnError {
			return fmt.Errorf("job %q step %d: timeouts and continue-on-error are not supported by CI import", name, i+1)
		}
		if _, err := conditionMatchesOS(step.If, "linux"); err != nil {
			return fmt.Errorf("job %q step %d: %w", name, i+1, err)
		}
		if err := rejectActionsExpression(step.Run); err != nil {
			return fmt.Errorf("job %q step %d run: %w", name, i+1, err)
		}
		if err := validateWorkflowEnv(step.Env); err != nil {
			return fmt.Errorf("job %q step %d: %w", name, i+1, err)
		}
		if err := validateWorkflowWorkingDirectory(step.WorkingDirectory); err != nil {
			return fmt.Errorf("job %q step %d working-directory: %w", name, i+1, err)
		}
		if err := validateWorkflowShell(step.Shell); err != nil {
			return fmt.Errorf("job %q step %d: %w", name, i+1, err)
		}
		if strings.TrimSpace(step.Run) != "" && strings.TrimSpace(step.Uses) != "" {
			return fmt.Errorf("job %q step %d: cannot combine run and uses", name, i+1)
		}
		for key, val := range step.With {
			if err := rejectActionsExpression(fmt.Sprint(val)); err != nil {
				return fmt.Errorf("job %q step %d with.%s: %w", name, i+1, key, err)
			}
		}
		if step.Uses != "" {
			if err := rejectActionsExpression(step.Uses); err != nil {
				return fmt.Errorf("job %q step %d uses: %w", name, i+1, err)
			}
			action := normalizeUsesAction(step.Uses)
			if action == "" || !supportedAction(action) {
				return fmt.Errorf("job %q step %d: unsupported action %q", name, i+1, step.Uses)
			}
		}
	}
	return nil
}

func isMatrixOSReference(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "${{") || !strings.HasSuffix(value, "}}") {
		return false
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${{"), "}}")) == "matrix.os"
}

func isSupportedRunner(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "ubuntu-latest" || value == "windows-latest" || value == "macos-latest" ||
		strings.HasPrefix(value, "ubuntu-") || strings.HasPrefix(value, "windows-") || strings.HasPrefix(value, "macos-")
}

func conditionMatchesOS(expr string, targetOS string) (bool, error) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "${{") && strings.HasSuffix(expr, "}}") {
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expr, "${{"), "}}"))
	}
	switch expr {
	case "", "true":
		return true, nil
	case "false":
		return false, nil
	}
	matches := simpleOSCondition.FindStringSubmatch(expr)
	if len(matches) != 3 || !isSupportedRunner(matches[2]) && !isOSName(matches[2]) {
		return false, fmt.Errorf("unsupported if condition %q", expr)
	}
	result := osFromValue(matches[2]) == targetOS
	if matches[1] == "!=" {
		return !result, nil
	}
	return result, nil
}

func isOSName(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "windows", "linux", "macos":
		return true
	default:
		return false
	}
}

func supportedAction(action string) bool {
	switch action {
	case "actions/checkout", "actions/cache", "actions/cache/restore", "actions/cache/save",
		"actions/setup-node", "actions/setup-go", "actions/setup-python",
		"actions/setup-dotnet", "actions/setup-java", "actions/setup-ruby", "ruby/setup-ruby":
		return true
	default:
		return false
	}
}

func validateWorkflowEnv(env map[string]interface{}) error {
	for name, raw := range env {
		switch raw.(type) {
		case string, bool, int, int64, float64:
		default:
			return fmt.Errorf("env %q: expected a scalar value", name)
		}
		if err := rejectActionsExpression(fmt.Sprint(raw)); err != nil {
			return fmt.Errorf("env %q: %w", name, err)
		}
	}
	return nil
}

func validateWorkflowShell(value string) error {
	if value == "" {
		return nil
	}
	if err := rejectActionsExpression(value); err != nil {
		return fmt.Errorf("shell: %w", err)
	}
	switch shell.Normalize(value) {
	case "bash", "sh", "pwsh", "powershell", "cmd":
		return nil
	default:
		return fmt.Errorf("unsupported workflow shell %q", value)
	}
}

func rejectActionsExpression(value string) error {
	if strings.Contains(value, "${{") || strings.Contains(value, "}}") {
		return fmt.Errorf("unsupported GitHub Actions expression in %q", value)
	}
	return nil
}

func isWorkspaceDirectory(value string) bool {
	normalized := normalizeWorkingDirectory(value)
	return normalized != strings.TrimSpace(value) && !strings.Contains(normalized, "${{")
}
