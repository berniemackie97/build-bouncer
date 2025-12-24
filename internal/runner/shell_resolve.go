package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
func resolveCommand(shell string, command string, fallbackShell string) (string, []string) {
	if executableName, executableArgs, ok := commandForShell(shell, command); ok {
		return executableName, executableArgs
	}

	if executableName, executableArgs, ok := directShellCommand(command); ok {
		return preferWindowsShell(executableName), executableArgs
	}

	if executableName, executableArgs, ok := commandForShell(fallbackShell, command); ok {
		return executableName, executableArgs
	}

	return shellCommand(command)
}

// commandForShell builds an exec name and args for a configured shell.
// shellSpec can be just a name, a path, or a quoted path with optional extra args.
// We treat extra args as prefix args for the shell.
func commandForShell(shellSpec string, command string) (string, []string, bool) {
	trimmedShellSpec := strings.TrimSpace(shellSpec)
	if trimmedShellSpec == "" {
		return "", nil, false
	}

	shellExecutable, shellPrefixArgs := splitExecutableAndArgs(trimmedShellSpec)
	if strings.TrimSpace(shellExecutable) == "" {
		return "", nil, false
	}

	lowerBaseName := strings.ToLower(filepath.Base(shellExecutable))

	switch lowerBaseName {
	case "bash", "bash.exe":
		execName := shellExecutable
		if shellExecutable == lowerBaseName {
			execName = preferWindowsShell("bash")
		}
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, "-lc", command)
		return execName, args, true

	case "sh", "sh.exe":
		execName := shellExecutable
		if shellExecutable == lowerBaseName {
			execName = preferWindowsShell("sh")
		}
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, "-c", command)
		return execName, args, true

	case "pwsh", "pwsh.exe":
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, "-NoProfile", "-NonInteractive", "-Command", command)
		return shellExecutable, args, true

	case "powershell", "powershell.exe":
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, "-NoProfile", "-NonInteractive", "-Command", command)
		return shellExecutable, args, true

	case "cmd", "cmd.exe":
		// cmd is special. We always use cmd.exe and then append any prefix args before /C.
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, "/C", command)
		return "cmd.exe", args, true

	default:
		// Unknown shell runner. We treat it as: <shell> <prefixArgs...> <command>
		args := append([]string{}, shellPrefixArgs...)
		args = append(args, command)
		return shellExecutable, args, true
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

	lowerExecutableBase := strings.ToLower(filepath.Base(executableToken))
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

	// Tests expect the executable name to be the canonical shell name, not the full path token.
	return expectedShell, []string{flagToken, script}, true
}

func parseDirectPowerShellInvocation(command string, expectedShell string) (string, []string, bool) {
	executableToken, remainingText := cutFirstToken(command)
	if executableToken == "" {
		return "", nil, false
	}

	lowerExecutableBase := strings.ToLower(filepath.Base(executableToken))
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
			return expectedShell, args, true
		}

		prefixArgs = append(prefixArgs, nextToken)
		remaining = strings.TrimSpace(rest)
		if remaining == "" {
			return "", nil, false
		}
	}
}

// splitExecutableAndArgs handles simple shell specs like:
// pwsh
// "C:\Program Files\PowerShell\7\pwsh.exe" -NoProfile
// 'C:\Program Files\PowerShell\7\pwsh.exe' -NoProfile
func splitExecutableAndArgs(spec string) (string, []string) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return "", nil
	}

	// If the spec starts with a quote, we treat it as "quoted executable path".
	// If the quote is unterminated, tests expect we treat the entire string as the executable.
	first := trimmed[0]
	if first == '"' || first == '\'' {
		quote := first
		rest := trimmed[1:]
		closing := strings.IndexByte(rest, quote)
		if closing == -1 {
			return trimmed, nil
		}

		executable := rest[:closing]
		after := strings.TrimSpace(trimmed[closing+2:])
		if after == "" {
			return executable, nil
		}

		// Prefix args are treated as whitespace separated fields.
		return executable, strings.Fields(after)
	}

	// Unquoted. First field is executable.
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) == 1 {
		return fields[0], nil
	}
	return fields[0], fields[1:]
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
