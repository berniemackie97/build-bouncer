package runner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func resolveDefaultLogDir(repoRoot string) string {
	if gitDir, ok := resolveGitDir(repoRoot); ok {
		return filepath.Join(gitDir, "build-bouncer", "logs")
	}
	return filepath.Join(repoRoot, config.ConfigDirName, "logs")
}

// resolveGitDir returns the real git directory for this repo root.
// It supports two layouts:
// 1) Normal repo where .git is a directory
// 2) Worktrees and submodules where .git is a file that contains: gitdir: <path>
func resolveGitDir(repoRoot string) (string, bool) {
	dotGitPath := filepath.Join(repoRoot, ".git")

	st, err := os.Stat(dotGitPath)
	if err != nil {
		return "", false
	}

	// Normal repo layout
	if st.IsDir() {
		return dotGitPath, true
	}

	// Worktrees and submodules often use a .git file that points to the real git dir
	raw, err := os.ReadFile(dotGitPath)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(raw), "\n")
	firstNonEmptyLine := ""
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line != "" {
			firstNonEmptyLine = line
			break
		}
	}
	if firstNonEmptyLine == "" {
		return "", false
	}

	lowerLine := strings.ToLower(firstNonEmptyLine)
	const prefix = "gitdir:"
	if !strings.HasPrefix(lowerLine, prefix) {
		return "", false
	}

	// Keep original casing for the path after gitdir:
	pathText := strings.TrimSpace(firstNonEmptyLine[len(prefix):])
	if pathText == "" {
		return "", false
	}

	// Git may store relative paths here. They are relative to the repo root where .git lives.
	gitDir := pathText
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	st, err = os.Stat(gitDir)
	if err != nil || !st.IsDir() {
		return "", false
	}

	return gitDir, true
}

// sanitize turns a user or config supplied name into something safe for filenames.
// This is intentionally conservative. If we cannot trust a character, we replace it.
func sanitize(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "check"
	}

	var builder strings.Builder
	builder.Grow(len(trimmed))

	for _, runeValue := range trimmed {
		switch {
		case runeValue >= 'a' && runeValue <= 'z':
			builder.WriteRune(runeValue)
		case runeValue >= 'A' && runeValue <= 'Z':
			builder.WriteRune(runeValue)
		case runeValue >= '0' && runeValue <= '9':
			builder.WriteRune(runeValue)
		case runeValue == '-' || runeValue == '_' || runeValue == '.':
			builder.WriteRune(runeValue)
		default:
			builder.WriteRune('_')
		}
	}

	return builder.String()
}
