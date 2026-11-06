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

// shellCommand is the final fallback when we cannot confidently resolve a specific shell.
func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

// resolveCommand decides how to run a check command.
// Order matters and is intentionally stable:
// 1. Respect explicit shell
// 2. Respect direct shell invocation in the command (bash -c "...", powershell -Command "...")
// 3. Respect fallback shell
// 4. Fall back to OS default
func resolveCommand(shellSpec string, command string, fallbackShell string) (string, []string, error) {
	if strings.TrimSpace(shellSpec) != "" {
		return commandForShell(shellSpec, command)
	}

	if executable, args, ok := directShellCommand(command); ok {
		return preferWindowsShell(executable), args, nil
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
			executable = preferWindowsShell("bash")
		}
		return executable, []string{"-lc", command}, nil
	case "sh", "sh.exe":
		if !strings.ContainsAny(executable, "/\\") {
			executable = preferWindowsShell("sh")
		}
		return executable, []string{"-c", command}, nil
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		return executable, []string{"-NoProfile", "-NonInteractive", "-Command", command}, nil
	case "cmd", "cmd.exe":
		if !strings.ContainsAny(executable, "/\\") {
			executable = "cmd.exe"
		}
		return executable, []string{"/C", command}, nil
	default:
		return executable, []string{command}, nil
	}
}

// directShellCommand detects a command that already explicitly calls a shell.
// Examples:
//
//	bash -lc "make test"
//	powershell -NoProfile -Command "Start-Sleep -Seconds 2"
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

	// This matters on Windows: wrapping a PowerShell call inside cmd.exe /C can get weird with quoting.
	// If the command is already a PowerShell invocation, run PowerShell directly.
	if executableName, executableArgs, ok := parseDirectPowerShellInvocation(trimmedCommand, "pwsh"); ok {
		return executableName, executableArgs, true
	}
	if executableName, executableArgs, ok := parseDirectPowerShellInvocation(trimmedCommand, "powershell"); ok {
		return executableName, executableArgs, true
	}

	return "", nil, false
}

// parseDirectShellInvocation detects a command that already explicitly calls a POSIX shell.
// Example: bash -lc "make test"
// Note: Tests expect this function to exist by this exact name.
func parseDirectShellInvocation(command string, expectedShell string) (string, []string, bool) {
	executableToken, remainingText := cutFirstToken(command)
	if executableToken == "" {
		return "", nil, false
	}

	lowerExecutableBase := strings.ToLower(shell.BaseName(executableToken))
	expectedLower := strings.ToLower(expectedShell)

	// Allow bash, bash.exe, sh, sh.exe, or full paths that end with those.
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

	// Support a quoted first token so paths with spaces work.
	// Tests require unterminated quotes to return empty.
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

	// Rebuild the rest without trying to be clever about quoting.
	// This is only used for direct invocation detection where we already require quotes around the script.
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

func preferWindowsShell(shell string) string {
	if runtime.GOOS != "windows" {
		return shell
	}

	shellPath, err := exec.LookPath(shell)
	if err != nil {
		return shell
	}

	if detectShellFlavorPath(shellPath) == pathFlavorWSL {
		if alt := findGitShell(shell); alt != "" {
			return alt
		}
	}

	return shellPath
}

// adjustEnvForShell exists because Windows path separators and bash path separators do not match.
// If we run bash or sh on Windows, we rewrite PATH into a form those shells usually understand.
func adjustEnvForShell(shell string, env []string) []string {
	if runtime.GOOS != "windows" {
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
	name := shell + ".exe"
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "usr", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", name),
	}

	for _, candidatePath := range candidates {
		if candidatePath == "" {
			continue
		}
		if fileExists(candidatePath) {
			return candidatePath
		}
	}

	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
