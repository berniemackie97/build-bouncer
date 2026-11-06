package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/shell"
)

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func resolveCommand(shellSpec string, command string, fallbackShell string) (string, []string, error) {
	if strings.TrimSpace(shellSpec) != "" {
		return commandForShell(shellSpec, command)
	}

	if executable, args, ok := directShellCommand(command); ok {
		selected, err := preferWindowsShell(executable)
		return selected, args, err
	}

	if strings.TrimSpace(fallbackShell) != "" {
		return commandForShell(fallbackShell, command)
	}

	executable, args := shellCommand(command)
	return executable, args, nil
}

func commandForShell(spec string, command string) (string, []string, error) {
	executable, err := shell.ParseExecutable(spec)
	if err != nil {
		return "", nil, fmt.Errorf("shell %q: %w", spec, err)
	}
	if executable == "" {
		return "", nil, errors.New("shell executable is required")
	}

	switch strings.ToLower(shell.BaseName(executable)) {
	case "bash", "bash.exe":
		if !strings.ContainsAny(executable, "/\\") {
			executable, err = preferWindowsShell(executable)
			if err != nil {
				return "", nil, err
			}
		}
		return executable, []string{"-lc", command}, nil
	case "sh", "sh.exe":
		if !strings.ContainsAny(executable, "/\\") {
			executable, err = preferWindowsShell(executable)
			if err != nil {
				return "", nil, err
			}
		}
		return executable, []string{"-c", command}, nil
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		return executable, []string{"-NoProfile", "-NonInteractive", "-Command", command}, nil
	case "wsl", "wsl.exe":
		if runtime.GOOS != "windows" {
			return "", nil, errors.New("WSL shell is supported only on Windows")
		}
		return executable, []string{"--exec", "sh", "-c", command}, nil
	case "cmd", "cmd.exe":
		if !strings.ContainsAny(executable, "/\\") {
			executable = "cmd.exe"
		}
		return executable, []string{"/C", command}, nil
	default:
		return executable, []string{command}, nil
	}
}

func directShellCommand(command string) (string, []string, bool) {
	trimmedCommand := strings.TrimSpace(command)
	if trimmedCommand == "" {
		return "", nil, false
	}

	if executableName, executableArgs, ok := parseDirectShellInvocation(trimmedCommand, "bash"); ok {
		return executableName, executableArgs, true
	}
	if executableName, executableArgs, ok := parseDirectShellInvocation(trimmedCommand, "sh"); ok {
		return executableName, executableArgs, true
	}

	if executableName, executableArgs, ok := parseDirectPowerShellInvocation(trimmedCommand, "pwsh"); ok {
		return executableName, executableArgs, true
	}
	if executableName, executableArgs, ok := parseDirectPowerShellInvocation(trimmedCommand, "powershell"); ok {
		return executableName, executableArgs, true
	}

	return "", nil, false
}

func parseDirectShellInvocation(command string, expectedShell string) (string, []string, bool) {
	executableToken, remainingText := cutFirstToken(command)
	if executableToken == "" {
		return "", nil, false
	}

	lowerExecutableBase := strings.ToLower(shell.BaseName(executableToken))
	expectedLower := strings.ToLower(expectedShell)

	if lowerExecutableBase != expectedLower && lowerExecutableBase != expectedLower+".exe" {
		return "", nil, false
	}

	flagToken, remainingAfterFlag := cutFirstToken(strings.TrimSpace(remainingText))
	if flagToken != "-lc" && flagToken != "-c" {
		return "", nil, false
	}

	scriptToken := strings.TrimSpace(remainingAfterFlag)
	if scriptToken == "" {
		return "", nil, false
	}

	script, ok := unquoteShellArg(scriptToken)
	if !ok {
		return "", nil, false
	}

	return executableToken, []string{flagToken, script}, true
}

func parseDirectPowerShellInvocation(command string, expectedShell string) (string, []string, bool) {
	executableToken, remainingText := cutFirstToken(command)
	if executableToken == "" {
		return "", nil, false
	}

	lowerExecutableBase := strings.ToLower(shell.BaseName(executableToken))
	expectedLower := strings.ToLower(expectedShell)

	if lowerExecutableBase != expectedLower && lowerExecutableBase != expectedLower+".exe" {
		return "", nil, false
	}

	remaining := strings.TrimSpace(remainingText)
	if remaining == "" {
		return "", nil, false
	}

	prefixArgs := make([]string, 0, 6)

	for {
		nextToken, rest := cutFirstToken(remaining)
		if nextToken == "" {
			return "", nil, false
		}

		lowerToken := strings.ToLower(strings.TrimSpace(nextToken))
		if lowerToken == "-command" || lowerToken == "-c" {
			scriptToken := strings.TrimSpace(rest)
			if scriptToken == "" {
				return "", nil, false
			}

			// PowerShell treats the remainder as the command text. Accept quoted or unquoted.
			scriptText := strings.TrimSpace(scriptToken)
			if unquoted, ok := unquoteShellArg(scriptText); ok {
				scriptText = unquoted
			}

			args := append([]string{}, prefixArgs...)
			args = append(args, nextToken, scriptText)
			return executableToken, args, true
		}

		prefixArgs = append(prefixArgs, nextToken)
		remaining = strings.TrimSpace(rest)
		if remaining == "" {
			return "", nil, false
		}
	}
}

func cutFirstToken(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	first := trimmed[0]
	if first == '"' || first == '\'' {
		quote := first
		rest := trimmed[1:]
		closingQuote := strings.IndexByte(rest, quote)
		if closingQuote == -1 {
			return "", ""
		}

		token := rest[:closingQuote]
		remaining := strings.TrimSpace(trimmed[closingQuote+2:])
		return token, remaining
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}

	firstToken := fields[0]
	rest := strings.TrimSpace(trimmed[len(firstToken):])
	return firstToken, rest
}

func unquoteShellArg(arg string) (string, bool) {
	trimmed := strings.TrimSpace(arg)
	if len(trimmed) < 2 {
		return "", false
	}

	quote := trimmed[0]
	if (quote != '"' && quote != '\'') || trimmed[len(trimmed)-1] != quote {
		return "", false
	}

	// Double quotes can be safely handled by strconv.Unquote.
	if quote == '"' {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			return unquoted, true
		}
		return trimmed[1 : len(trimmed)-1], true
	}

	// Single quotes are treated as literal content.
	return trimmed[1 : len(trimmed)-1], true
}

func preferWindowsShell(spec string) (string, error) {
	if runtime.GOOS != "windows" || strings.ContainsAny(spec, "/\\") {
		return spec, nil
	}

	name := strings.ToLower(strings.TrimSuffix(strings.ToLower(spec), ".exe"))
	if name != "bash" && name != "sh" {
		return spec, nil
	}

	resolved, err := exec.LookPath(spec)
	if err != nil {
		if gitShell := findGitShell(name); gitShell != "" {
			return gitShell, nil
		}
		return "", fmt.Errorf("locate %s shell: %w; install Git Bash or configure an explicit executable path", spec, err)
	}
	if detectShellFlavorPath(resolved) == pathFlavorWSL {
		if gitShell := findGitShell(name); gitShell != "" {
			return gitShell, nil
		}
		return "", fmt.Errorf("%s resolves to the WSL launcher %q; configure an explicit WSL executable path or install Git Bash", spec, resolved)
	}
	return resolved, nil
}

// ResolveCheckExecutable reports the executable selected for a check without running it.
func ResolveCheckExecutable(shellSpec, command string) (string, error) {
	return ResolveCheckExecutableAt("", shellSpec, command, nil)
}

// ResolveCheckExecutableAt resolves project-relative executables against repoRoot.
func ResolveCheckExecutableAt(repoRoot, shellSpec, command string, env map[string]string) (string, error) {
	executable, _, err := resolveCommand(shellSpec, command, "")
	if err != nil {
		return "", err
	}
	executable = resolveExecutableAt(repoRoot, executable)
	path, err := exec.LookPath(executable)
	if err != nil {
		return "", fmt.Errorf("executable %q is unavailable: %w", executable, err)
	}
	if err := validateWSLEnvironment(path, env); err != nil {
		return "", err
	}
	return path, nil
}

func resolveExecutableAt(repoRoot, executable string) string {
	if repoRoot != "" && strings.ContainsAny(executable, "/\\") && !filepath.IsAbs(executable) &&
		!(len(executable) >= 2 && executable[1] == ':') {
		return filepath.Join(repoRoot, executable)
	}
	return executable
}

func isWSLLauncher(executable string) bool {
	switch strings.ToLower(shell.BaseName(executable)) {
	case "wsl", "wsl.exe":
		return true
	case "bash", "bash.exe":
		return detectShellFlavorPath(executable) == pathFlavorWSL
	default:
		return false
	}
}

func validateWSLEnvironment(executable string, overrides map[string]string) error {
	if runtime.GOOS == "windows" && len(overrides) > 0 && isWSLLauncher(executable) {
		return errors.New("WSL shell does not support check env overrides; configure environment inside the Linux command")
	}
	return nil
}

// adjustEnvForShell exists because Windows path separators and bash path separators do not match.
// If we run bash or sh on Windows, we rewrite PATH into a form those shells usually understand.
func adjustEnvForShell(shell string, env []string) []string {
	if runtime.GOOS != "windows" {
		return env
	}

	if isWSLLauncher(shell) {
		return env
	}

	lowerBase := strings.ToLower(filepath.Base(shell))
	if lowerBase != "bash" && lowerBase != "bash.exe" && lowerBase != "sh" && lowerBase != "sh.exe" {
		return fixWindowsPathFromPosix(env)
	}

	return fixBashPath(env, detectShellFlavor(shell))
}

type shellPathFlavor int

const (
	pathFlavorMSYS shellPathFlavor = iota
	pathFlavorWSL
)

func detectShellFlavor(shell string) shellPathFlavor {
	if strings.ContainsAny(shell, "/\\") {
		return detectShellFlavorPath(shell)
	}
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return pathFlavorMSYS
	}
	return detectShellFlavorPath(shellPath)
}

func detectShellFlavorPath(path string) shellPathFlavor {
	lower := strings.ToLower(path)

	if strings.Contains(lower, `\system32\bash.exe`) ||
		strings.Contains(lower, `\system32\wsl.exe`) ||
		strings.Contains(lower, `\windowsapps\bash.exe`) ||
		strings.Contains(lower, `\windowsapps\wsl.exe`) {
		return pathFlavorWSL
	}

	if strings.Contains(lower, `\git\`) || strings.Contains(lower, `\msys`) || strings.Contains(lower, `\mingw`) {
		return pathFlavorMSYS
	}

	return pathFlavorMSYS
}

func findGitShell(shell string) string {
	name := strings.TrimSuffix(strings.ToLower(shell), ".exe") + ".exe"
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root == "" {
			continue
		}
		for _, directory := range []string{"usr", ""} {
			candidate := filepath.Join(root, "Git", directory, "bin", name)
			if fileExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
