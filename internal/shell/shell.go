package shell

import (
	"path/filepath"
	"strings"
)

type ScriptType int

const (
	ScriptUnknown ScriptType = iota
	ScriptPosix
	ScriptPowerShell
)

func DetectScriptType(command string) ScriptType {
	if looksLikePowerShell(command) {
		return ScriptPowerShell
	}
	if looksLikePosixShell(command) {
		return ScriptPosix
	}
	return ScriptUnknown
}

func Resolve(goos string, preferred string, command string) string {
	if s := Normalize(preferred); s != "" {
		return s
	}
	switch DetectScriptType(command) {
	case ScriptPowerShell:
		return "pwsh"
	case ScriptPosix:
		if goos == "windows" {
			return "bash"
		}
		return "bash"
	default:
		return DefaultForOS(goos)
	}
}

func DefaultForOS(goos string) string {
	switch goos {
	case "windows":
		return "cmd"
	case "linux", "darwin":
		return "sh"
	default:
		return ""
	}
}

func Normalize(shell string) string {
	trimmed := strings.TrimSpace(shell)
	if trimmed == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(trimmed))
	switch base {
	case "bash", "bash.exe":
		return "bash"
	case "sh", "sh.exe":
		return "sh"
	case "pwsh", "pwsh.exe":
		return "pwsh"
	case "powershell", "powershell.exe":
		return "powershell"
	case "cmd", "cmd.exe":
		return "cmd"
	default:
		return trimmed
	}
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
