package runner

import (
	"path/filepath"
	"strings"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/git"
)

func resolveDefaultLogDir(repoRoot string) string {
	if gitDir, ok := git.ResolveGitDir(repoRoot); ok {
		return filepath.Join(gitDir, "build-bouncer", "logs")
	}
	return filepath.Join(repoRoot, config.ConfigDirName, "logs")
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
