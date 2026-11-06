package shell

import (
	"errors"
	"path"
	"strings"
)

// ParseExecutable accepts a shell name or executable path, never arguments.
// Quoted paths may contain spaces; unquoted paths with spaces must end in .exe.
func ParseExecutable(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", nil
	}
	if strings.ContainsAny(spec, "\x00\r\n\t") {
		return "", errors.New("shell executable must not contain control characters")
	}

	executable := spec
	if spec[0] == '"' || spec[0] == '\'' {
		if len(spec) < 2 || spec[len(spec)-1] != spec[0] {
			return "", errors.New("unterminated quoted shell executable")
		}
		executable = spec[1 : len(spec)-1]
	} else if strings.Contains(spec, " ") {
		extension := path.Ext(strings.ReplaceAll(spec, "\\", "/"))
		if !strings.ContainsAny(spec, "/\\") || !strings.EqualFold(extension, ".exe") {
			return "", errors.New("shell executable must not include arguments; quote paths containing spaces")
		}
	}
	if executable == "" || strings.HasPrefix(executable, "-") || strings.ContainsAny(executable, "\"'") {
		return "", errors.New("invalid shell executable")
	}
	return executable, nil
}

// BaseName handles Windows and POSIX path separators on every platform.
func BaseName(executable string) string {
	return path.Base(strings.ReplaceAll(executable, "\\", "/"))
}
