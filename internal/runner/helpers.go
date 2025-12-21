package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func resolveDefaultLogDir(repoRoot string) string {
	gitDir := filepath.Join(repoRoot, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return filepath.Join(gitDir, "build-bouncer", "logs")
	}
	return filepath.Join(repoRoot, ".buildbouncer", "logs")
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "check"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func resolveCommand(shell string, command string, fallbackShell string) (string, []string) {
	if name, args, ok := commandForShell(shell, command); ok {
		return name, args
	}
	if name, args, ok := directShellCommand(command); ok {
		return preferWindowsShell(name), args
	}
	if auto := detectShellForRun(command); auto != "" {
		if name, args, ok := commandForShell(auto, command); ok {
			return name, args
		}
	}
	if fallbackShell != "" {
		if name, args, ok := commandForShell(fallbackShell, command); ok {
			return name, args
		}
	}
	return shellCommand(command)
}

func commandForShell(shell string, command string) (string, []string, bool) {
	s := strings.TrimSpace(shell)
	if s == "" {
		return "", nil, false
	}
	base := strings.ToLower(filepath.Base(s))
	switch base {
	case "bash", "bash.exe":
		name := s
		if s == base {
			name = preferWindowsShell("bash")
		}
		return name, []string{"-lc", command}, true
	case "sh", "sh.exe":
		name := s
		if s == base {
			name = preferWindowsShell("sh")
		}
		return name, []string{"-c", command}, true
	case "pwsh", "pwsh.exe":
		return s, []string{"-NoProfile", "-NonInteractive", "-Command", command}, true
	case "powershell", "powershell.exe":
		return s, []string{"-NoProfile", "-NonInteractive", "-Command", command}, true
	case "cmd", "cmd.exe":
		return "cmd.exe", []string{"/C", command}, true
	default:
		return s, []string{command}, true
	}
}

func directShellCommand(command string) (string, []string, bool) {
	s := strings.TrimSpace(command)
	if s == "" {
		return "", nil, false
	}
	if name, args, ok := parseShellCommand("bash", "-lc", s); ok {
		return name, args, true
	}
	if name, args, ok := parseShellCommand("bash", "-c", s); ok {
		return name, args, true
	}
	if name, args, ok := parseShellCommand("sh", "-c", s); ok {
		return name, args, true
	}
	return "", nil, false
}

func parseShellCommand(shell string, flag string, command string) (string, []string, bool) {
	prefix := shell + " " + flag
	if !strings.HasPrefix(command, prefix) {
		return "", nil, false
	}
	rest := strings.TrimSpace(command[len(prefix):])
	if rest == "" {
		return "", nil, false
	}
	script, ok := unquoteShellArg(rest)
	if !ok {
		return "", nil, false
	}
	return shell, []string{flag, script}, true
}

func unquoteShellArg(arg string) (string, bool) {
	if len(arg) < 2 || arg[0] != '"' || arg[len(arg)-1] != '"' {
		return "", false
	}
	if s, err := strconv.Unquote(arg); err == nil {
		return s, true
	}
	return arg[1 : len(arg)-1], true
}

func detectShellForRun(command string) string {
	s := strings.TrimSpace(command)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "#!/usr/bin/env bash") || strings.HasPrefix(lower, "#!/bin/bash") {
		if hasShell("bash") {
			return "bash"
		}
	}
	if strings.HasPrefix(lower, "#!/usr/bin/env sh") || strings.HasPrefix(lower, "#!/bin/sh") {
		if hasShell("sh") {
			return "sh"
		}
	}
	if strings.HasPrefix(lower, "#!/usr/bin/env pwsh") || strings.HasPrefix(lower, "#!/usr/bin/pwsh") {
		if hasShell("pwsh") {
			return "pwsh"
		}
	}
	if strings.HasPrefix(lower, "#!/usr/bin/env powershell") {
		if hasShell("powershell") {
			return "powershell"
		}
	}
	if looksLikePosixShell(s) {
		if hasShell("bash") {
			return "bash"
		}
		if hasShell("sh") {
			return "sh"
		}
	}
	if looksLikePowerShell(s) {
		if hasShell("pwsh") {
			return "pwsh"
		}
		if hasShell("powershell") {
			return "powershell"
		}
	}
	return ""
}

func looksLikePosixShell(command string) bool {
	if !strings.Contains(command, "\n") && !strings.Contains(command, "\r\n") {
		return looksLikePosixOneLiner(command)
	}
	lower := strings.ToLower(command)
	if strings.Contains(lower, "set -e") || strings.Contains(lower, "set -euo") {
		return true
	}
	if strings.Contains(lower, "pipefail") || strings.Contains(lower, "if [") || strings.Contains(lower, "[[") {
		return true
	}
	if strings.Contains(lower, "\nfi") || strings.Contains(lower, "\nthen") {
		return true
	}
	for _, line := range strings.Split(command, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return looksLikePosixOneLiner(line)
	}
	return false
}

func looksLikePosixOneLiner(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	first := fields[0]
	if strings.HasPrefix(first, "./") || strings.HasPrefix(first, "../") {
		return true
	}
	if strings.HasPrefix(first, "/") && !looksLikeWindowsDrivePath(first) {
		return true
	}
	if strings.HasSuffix(first, ".sh") {
		return true
	}
	if _, ok := posixOnlyCommands[first]; ok {
		return true
	}
	return false
}

func looksLikeWindowsDrivePath(token string) bool {
	if len(token) < 2 {
		return false
	}
	return token[1] == ':' && ((token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z'))
}

var posixOnlyCommands = map[string]struct{}{
	"awk":      {},
	"basename": {},
	"cat":      {},
	"chgrp":    {},
	"chmod":    {},
	"chown":    {},
	"cp":       {},
	"curl":     {},
	"dirname":  {},
	"find":     {},
	"grep":     {},
	"gunzip":   {},
	"gzip":     {},
	"head":     {},
	"ln":       {},
	"ls":       {},
	"mkdir":    {},
	"mv":       {},
	"pwd":      {},
	"readlink": {},
	"realpath": {},
	"rm":       {},
	"rmdir":    {},
	"sed":      {},
	"tail":     {},
	"tar":      {},
	"touch":    {},
	"unzip":    {},
	"wget":     {},
	"xargs":    {},
	"zip":      {},
}

func looksLikePowerShell(command string) bool {
	if command == "" {
		return false
	}
	lower := strings.ToLower(command)
	if strings.Contains(lower, "$env:") || strings.Contains(lower, "$psstyle") || strings.Contains(lower, "$pshome") {
		return true
	}
	if strings.Contains(lower, "$erroractionpreference") || strings.Contains(lower, "set-strictmode") {
		return true
	}
	if strings.Contains(lower, "write-host") || strings.Contains(lower, "write-output") {
		return true
	}
	if strings.Contains(lower, "get-childitem") || strings.Contains(lower, "get-item") || strings.Contains(lower, "set-location") {
		return true
	}
	if strings.Contains(lower, "join-path") || strings.Contains(lower, "test-path") {
		return true
	}
	if strings.Contains(lower, "param(") || strings.Contains(lower, "function ") {
		return true
	}
	if strings.Contains(lower, "if (") && strings.Contains(lower, "{") {
		return true
	}
	return false
}

func defaultShellForCheck(checkName string) string {
	if strings.HasPrefix(checkName, "ci:") {
		return defaultCIShell()
	}
	return ""
}

func defaultCIShell() string {
	switch runtime.GOOS {
	case "windows":
		if hasShell("pwsh") {
			return "pwsh"
		}
		if hasShell("powershell") {
			return "powershell"
		}
		return "cmd"
	case "darwin", "linux":
		if hasShell("bash") {
			return "bash"
		}
		if hasShell("sh") {
			return "sh"
		}
		return ""
	default:
		return ""
	}
}

func hasShell(shell string) bool {
	if runtime.GOOS == "windows" && (shell == "bash" || shell == "sh") {
		if path, err := exec.LookPath(shell); err == nil {
			if detectShellFlavorPath(path) == pathFlavorWSL {
				return findGitShell(shell) != ""
			}
			return true
		}
		return findGitShell(shell) != ""
	}
	_, err := exec.LookPath(shell)
	return err == nil
}

func preferWindowsShell(shell string) string {
	if runtime.GOOS != "windows" {
		return shell
	}
	path, err := exec.LookPath(shell)
	if err != nil {
		return shell
	}
	if detectShellFlavorPath(path) == pathFlavorWSL {
		if alt := findGitShell(shell); alt != "" {
			return alt
		}
	}
	return path
}

func adjustEnvForShell(shell string, env []string) []string {
	if runtime.GOOS != "windows" {
		return env
	}
	base := strings.ToLower(filepath.Base(shell))
	if base != "bash" && base != "bash.exe" && base != "sh" && base != "sh.exe" {
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
	path, err := exec.LookPath(shell)
	if err != nil {
		return pathFlavorMSYS
	}
	return detectShellFlavorPath(path)
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

func fixBashPath(env []string, flavor shellPathFlavor) []string {
	idx := -1
	path := ""
	for i, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			idx = i
			path = kv[len("PATH="):]
			break
		}
	}
	if idx == -1 || path == "" {
		return env
	}
	entries := filepath.SplitList(path)
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.Trim(entry, "\"")
		if entry == "" {
			continue
		}
		out = append(out, toShellPath(entry, flavor))
	}
	env[idx] = "PATH=" + strings.Join(out, ":")
	return env
}

func fixWindowsPathFromPosix(env []string) []string {
	idx := -1
	path := ""
	for i, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			idx = i
			path = kv[len("PATH="):]
			break
		}
	}
	if idx == -1 || path == "" {
		return env
	}
	if strings.Contains(path, ";") {
		return env
	}
	if !looksLikePosixPathList(path) {
		return env
	}

	entries := strings.Split(path, ":")
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.Trim(entry, "\"")
		if entry == "" {
			continue
		}
		if win, ok := posixToWindowsPath(entry); ok {
			out = append(out, win)
			continue
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return env
	}
	env[idx] = "PATH=" + strings.Join(out, ";")
	return env
}

func looksLikePosixPathList(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	}
	return strings.Contains(path, ":/")
}

func posixToWindowsPath(path string) (string, bool) {
	if len(path) < 3 || path[0] != '/' {
		return "", false
	}
	drive := path[1]
	if !((drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')) || path[2] != '/' {
		return "", false
	}
	rest := path[3:]
	rest = strings.ReplaceAll(rest, "/", "\\")
	return strings.ToUpper(string(drive)) + ":\\" + rest, true
}

func toShellPath(path string, flavor shellPathFlavor) string {
	p := strings.ReplaceAll(path, "\\", "/")
	if strings.HasPrefix(p, "/") {
		return p
	}
	if len(p) >= 2 && p[1] == ':' {
		drive := strings.ToLower(p[:1])
		rest := p[2:]
		if !strings.HasPrefix(rest, "/") {
			rest = "/" + rest
		}
		if flavor == pathFlavorWSL {
			return "/mnt/" + drive + rest
		}
		return "/" + drive + rest
	}
	return p
}

func findGitShell(shell string) string {
	name := shell + ".exe"
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "usr", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "usr", "bin", name),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", name),
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func TailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
