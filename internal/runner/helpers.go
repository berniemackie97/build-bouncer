package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func resolveDefaultLogDir(repoRoot string) string {
	gitDir := filepath.Join(repoRoot, ".git")
	if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
		return filepath.Join(gitDir, "build-bouncer", "logs")
	}
	return filepath.Join(repoRoot, ".build-bouncer", "logs")
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
