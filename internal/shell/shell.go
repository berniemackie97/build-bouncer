// Package shell contains helpers for deciding which shell a command probably expects.
package shell

import (
	"path/filepath"
	"slices"
	"strings"
)

// ScriptType describes what kind of shell a command looks like.
type ScriptType int

const (
	ScriptUnknown ScriptType = iota
	ScriptPosix
	ScriptPowerShell
)

// We use different thresholds depending on whether the command is multi line or single line.
// Multi line commands usually represent scripts, so we can be a bit more willing to classify.
// Single line commands are often ambiguous, so we require strong evidence.
const (
	minimumScoreToChooseMultiLine  = 2
	minimumScoreToChooseSingleLine = 3
)

// DetectScriptType tries to guess what shell a command is written for.
func DetectScriptType(command string) ScriptType {
	trimmedCommand := strings.TrimSpace(command)
	if trimmedCommand == "" {
		return ScriptUnknown
	}

	normalizedCommand := normalizeNewlines(trimmedCommand)

	shebangType := detectShebangScriptType(normalizedCommand)
	if shebangType != ScriptUnknown {
		return shebangType
	}

	isMultiLineCommand := strings.Contains(normalizedCommand, "\n")

	posixScore := scorePosix(normalizedCommand, isMultiLineCommand)
	powerShellScore := scorePowerShell(normalizedCommand, isMultiLineCommand)

	minimumScoreToChoose := minimumScoreToChooseSingleLine
	if isMultiLineCommand {
		minimumScoreToChoose = minimumScoreToChooseMultiLine
	}

	if powerShellScore >= minimumScoreToChoose && powerShellScore > posixScore {
		return ScriptPowerShell
	}
	if posixScore >= minimumScoreToChoose && posixScore > powerShellScore {
		return ScriptPosix
	}

	return ScriptUnknown
}

// Resolve returns the shell executable name to use.
// preferred wins if it is provided and recognizable.
// Otherwise we guess based on the command content, then fall back to OS defaults.
func Resolve(goos string, preferred string, command string) string {
	normalizedPreferredShell := Normalize(preferred)
	if normalizedPreferredShell != "" {
		return normalizedPreferredShell
	}

	switch DetectScriptType(command) {
	case ScriptPowerShell:
		return "pwsh"
	case ScriptPosix:
		return "bash"
	default:
		return DefaultForOS(goos)
	}
}

// DefaultForOS returns the default shell to use for a given GOOS.
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

// Normalize tries to normalize common shell names and paths into a stable executable name.
// If it does not recognize the input, it returns the trimmed string as is.
func Normalize(shell string) string {
	trimmedShell := strings.TrimSpace(shell)
	if trimmedShell == "" {
		return ""
	}

	lowerBaseName := strings.ToLower(filepath.Base(trimmedShell))

	switch lowerBaseName {
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
		return trimmedShell
	}
}

func normalizeNewlines(command string) string {
	withoutWindowsNewlines := strings.ReplaceAll(command, "\r\n", "\n")
	withoutCarriageReturns := strings.ReplaceAll(withoutWindowsNewlines, "\r", "\n")
	return withoutCarriageReturns
}

func detectShebangScriptType(command string) ScriptType {
	firstMeaningfulLine := firstNonEmptyLine(command)
	if !strings.HasPrefix(firstMeaningfulLine, "#!") {
		return ScriptUnknown
	}

	lowerShebangLine := strings.ToLower(firstMeaningfulLine)

	if strings.Contains(lowerShebangLine, "pwsh") || strings.Contains(lowerShebangLine, "powershell") {
		return ScriptPowerShell
	}

	if strings.Contains(lowerShebangLine, "bash") ||
		strings.Contains(lowerShebangLine, "/sh") ||
		strings.Contains(lowerShebangLine, "zsh") ||
		strings.Contains(lowerShebangLine, "dash") ||
		strings.Contains(lowerShebangLine, "ksh") {
		return ScriptPosix
	}

	return ScriptUnknown
}

func firstNonEmptyLine(command string) string {
	for rawLine := range strings.SplitSeq(command, "\n") {
		trimmedLine := strings.TrimSpace(rawLine)
		if trimmedLine == "" {
			continue
		}
		return trimmedLine
	}
	return ""
}

func scorePosix(command string, allowCommandNameHeuristics bool) int {
	score := 0

	lowerCommand := strings.ToLower(command)

	// Strong syntax hints. These are meaningful even for one liners.
	if strings.Contains(lowerCommand, "set -e") || strings.Contains(lowerCommand, "set -euo") {
		score += 2
	}
	if strings.Contains(lowerCommand, "pipefail") {
		score += 2
	}
	if strings.Contains(lowerCommand, "[[") || strings.Contains(lowerCommand, "if [") {
		score += 2
	}
	if strings.Contains(lowerCommand, "\nfi") || strings.Contains(lowerCommand, "\nthen") {
		score += 1
	}
	if strings.Contains(lowerCommand, "export ") {
		score += 1
	}
	if strings.Contains(lowerCommand, "$(") {
		score += 1
	}

	firstLine := firstNonEmptyLine(command)
	firstToken, remainingTokens := splitFirstToken(firstLine)
	if firstToken == "" {
		return score
	}

	// Explicit shell calls are strong evidence, even for one liners.
	if firstToken == "bash" || firstToken == "sh" || firstToken == "zsh" || firstToken == "dash" || firstToken == "ksh" {
		score += 3
		return score
	}

	// Path style hints.
	if strings.HasPrefix(firstToken, "./") || strings.HasPrefix(firstToken, "../") {
		score += 2
	}
	if strings.HasPrefix(firstToken, "/") && !looksLikeWindowsDrivePath(firstToken) {
		score += 2
	}
	if strings.HasSuffix(firstToken, ".sh") {
		score += 3
	}

	// Single line policy: do not guess based on command names or short flags.
	// If we do not have strong syntax or path based signal, we keep it unknown.
	if !allowCommandNameHeuristics {
		return score
	}

	// Multi line policy: allow weaker heuristics since we are likely looking at a script body.
	// Even here, we treat common PowerShell aliases as ambiguous unless there is extra unix looking signal.
	if _, isAmbiguous := ambiguousCommandNames[firstToken]; isAmbiguous {
		if containsUnixStyleFlagBundle(remainingTokens) {
			score += 1
		}
		return score
	}

	if _, isPosixLikely := posixLikelyCommandNames[firstToken]; isPosixLikely {
		score += 1
	}

	return score
}

func scorePowerShell(command string, allowCommandNameHeuristics bool) int {
	score := 0

	lowerCommand := strings.ToLower(command)

	// Strong PowerShell markers. These are meaningful even for one liners.
	if strings.Contains(lowerCommand, "$env:") {
		score += 3
	}
	if strings.Contains(lowerCommand, "$erroractionpreference") {
		score += 3
	}
	if strings.Contains(lowerCommand, "set-strictmode") {
		score += 2
	}
	if strings.Contains(lowerCommand, "param(") {
		score += 2
	}
	if strings.Contains(lowerCommand, "function ") {
		score += 2
	}

	// Literal patterns that strongly lean PowerShell.
	if strings.Contains(lowerCommand, "$true") || strings.Contains(lowerCommand, "$false") || strings.Contains(lowerCommand, "$null") {
		score += 2
	}
	if strings.Contains(lowerCommand, "$_") {
		score += 2
	}
	if strings.Contains(lowerCommand, "| foreach-object") || strings.Contains(lowerCommand, "| where-object") {
		score += 2
	}

	firstLine := firstNonEmptyLine(command)
	firstToken, remainingTokens := splitFirstToken(firstLine)
	if firstToken == "" {
		return score
	}

	// Explicit shell calls are strong evidence.
	if firstToken == "pwsh" || firstToken == "powershell" {
		score += 3
		return score
	}

	// Running a ps1 script is strong evidence.
	if strings.HasSuffix(firstToken, ".ps1") {
		score += 3
	}

	// Multi line policy: allow cmdlet name heuristics because scripts often use cmdlet verbs.
	// Single line policy: we still allow these, but they are not enough by themselves to beat the higher threshold.
	if allowCommandNameHeuristics {
		if strings.Contains(lowerCommand, "get-childitem") || strings.Contains(lowerCommand, "get-item") || strings.Contains(lowerCommand, "set-location") {
			score += 2
		}
		if strings.Contains(lowerCommand, "join-path") || strings.Contains(lowerCommand, "test-path") {
			score += 2
		}
		if strings.Contains(lowerCommand, "write-host") || strings.Contains(lowerCommand, "write-output") {
			score += 1
		}
	}

	// PowerShell parameter style is usually wordy, not unix bundles like -la.
	// This is a weak hint, but it can help in multi line scripts.
	if containsPowerShellStyleParameters(remainingTokens) {
		score += 1
	}

	return score
}

func splitFirstToken(line string) (string, []string) {
	trimmedLine := strings.TrimSpace(line)
	if trimmedLine == "" {
		return "", nil
	}

	fields := strings.Fields(trimmedLine)
	if len(fields) == 0 {
		return "", nil
	}

	firstTokenLower := strings.ToLower(fields[0])

	remainingTokensLower := make([]string, 0, len(fields)-1)
	for _, token := range fields[1:] {
		remainingTokensLower = append(remainingTokensLower, strings.ToLower(token))
	}

	return firstTokenLower, remainingTokensLower
}

func containsUnixStyleFlagBundle(tokens []string) bool {
	return slices.ContainsFunc(tokens, looksLikeUnixShortFlagBundle)
}

func looksLikeUnixShortFlagBundle(token string) bool {
	trimmedToken := strings.TrimSpace(token)
	if len(trimmedToken) < 3 {
		return false
	}
	if !strings.HasPrefix(trimmedToken, "-") || strings.HasPrefix(trimmedToken, "--") {
		return false
	}

	for characterIndex := 1; characterIndex < len(trimmedToken); characterIndex++ {
		currentByte := trimmedToken[characterIndex]
		if currentByte < 'a' || currentByte > 'z' {
			return false
		}
	}

	return true
}

func containsPowerShellStyleParameters(tokens []string) bool {
	for _, token := range tokens {
		if !strings.HasPrefix(token, "-") {
			continue
		}

		if token == "-noprofile" || token == "-executionpolicy" || token == "-erroraction" || token == "-literalpath" {
			return true
		}
	}

	return false
}

func looksLikeWindowsDrivePath(token string) bool {
	if len(token) < 2 {
		return false
	}
	isAlpha := (token[0] >= 'a' && token[0] <= 'z') || (token[0] >= 'A' && token[0] <= 'Z')
	return token[1] == ':' && isAlpha
}

// These command names are ambiguous because they are common PowerShell aliases.
var ambiguousCommandNames = map[string]struct{}{
	"cat":   {},
	"cp":    {},
	"ln":    {},
	"ls":    {},
	"mkdir": {},
	"mv":    {},
	"pwd":   {},
	"rm":    {},
	"touch": {},
}

// These command names are a hint that the author expects a POSIX like environment.
// This is weak evidence, so it is only used when we are already treating the input like a script body.
var posixLikelyCommandNames = map[string]struct{}{
	"awk":      {},
	"basename": {},
	"chgrp":    {},
	"chmod":    {},
	"chown":    {},
	"curl":     {},
	"dirname":  {},
	"find":     {},
	"grep":     {},
	"gunzip":   {},
	"gzip":     {},
	"head":     {},
	"readlink": {},
	"realpath": {},
	"rmdir":    {},
	"sed":      {},
	"tail":     {},
	"tar":      {},
	"unzip":    {},
	"wget":     {},
	"xargs":    {},
	"zip":      {},
}
