package runner

import (
	"path/filepath"
	"strings"
)

// cleanPathEntry removes surrounding double quotes from an env PATH entry.
// We do this because PATH entries sometimes come through like "C:\Program Files\Git\bin".
func cleanPathEntry(entry string) string {
	return strings.Trim(entry, "\"")
}

// fixBashPath rewrites PATH into a colon-separated list and converts Windows-style entries
// into the shell flavor you are targeting (ex: /c/... for Git Bash or /mnt/c/... for WSL).
//
// Important behavior:
//   - If PATH already looks like a posix list, we do not rewrite it again.
//     This avoids breaking mixed cases like C:/something:/usr/bin.
func fixBashPath(env []string, flavor shellPathFlavor) []string {
	pathIndex, pathValue := findEnvVar(env, "PATH")
	if pathIndex == -1 || pathValue == "" {
		return env
	}

	// If PATH already looks like a posix list, do not rewrite it again.
	// This avoids breaking cases like C:/something:/usr/bin
	if !strings.Contains(pathValue, ";") && looksLikePosixPathList(pathValue) {
		return env
	}

	entries := filepath.SplitList(pathValue)
	out := make([]string, 0, len(entries))

	for _, entry := range entries {
		cleaned := cleanPathEntry(entry)
		if cleaned == "" {
			continue
		}
		out = append(out, toShellPath(cleaned, flavor))
	}

	env[pathIndex] = "PATH=" + strings.Join(out, ":")
	return env
}

// fixWindowsPathFromPosix converts a colon-separated PATH list into a Windows-style
// semicolon-separated PATH list.
//
// Important behavior:
// - If PATH already contains semicolons, we assume it is already a Windows PATH list.
// - If the string does not look like a posix list, do not touch it (avoid false positives).
func fixWindowsPathFromPosix(env []string) []string {
	pathIndex, pathValue := findEnvVar(env, "PATH")
	if pathIndex == -1 || pathValue == "" {
		return env
	}

	// If we already have Windows PATH separators, do not touch it.
	if strings.Contains(pathValue, ";") {
		return env
	}

	// If it does not look like a real posix list, do not touch it.
	if !looksLikePosixPathList(pathValue) {
		return env
	}

	entries := strings.Split(pathValue, ":")
	out := make([]string, 0, len(entries))

	for _, entry := range entries {
		cleaned := cleanPathEntry(entry)
		if cleaned == "" {
			continue
		}

		// Convert /c/... or /mnt/c/... into C:\... when possible.
		// If it is not in that format, keep it as-is.
		if windowsPath, ok := posixToWindowsPath(cleaned); ok {
			out = append(out, windowsPath)
			continue
		}

		out = append(out, cleaned)
	}

	if len(out) == 0 {
		return env
	}

	env[pathIndex] = "PATH=" + strings.Join(out, ";")
	return env
}

// findEnvVar returns the index and value for the first matching env var entry.
// Matching is case-insensitive because Windows environment vars are case-insensitive,
// and we do not want to miss PATH vs Path vs path.
func findEnvVar(env []string, key string) (int, string) {
	for i, kv := range env {
		name, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(name, key) {
			return i, value
		}
	}
	return -1, ""
}

// looksLikePosixPathList tries to prevent false positives like "C:/Windows/System32".
// We only return true when there is evidence that colon is acting as a list separator.
//
// Why this exists:
// - On Windows you can see forward slashes, and "C:/Windows/System32" includes a colon.
// - We do not want to treat that as "two entries separated by :".
func looksLikePosixPathList(pathValue string) bool {
	if pathValue == "" {
		return false
	}

	colonCount := strings.Count(pathValue, ":")
	if colonCount == 0 {
		return false
	}

	firstColonIndex := strings.IndexByte(pathValue, ':')
	if colonCount == 1 && firstColonIndex == 1 && isAlphaASCII(pathValue[0]) {
		// This is probably a single Windows path using forward slashes, not a path list.
		return false
	}

	// If it starts with /, it is almost certainly a posix list or a single posix path.
	// Note: single posix paths typically have no colon, and we already checked colonCount.
	if strings.HasPrefix(pathValue, "/") {
		return true
	}

	// If it has more than one colon, it is likely using colon separators.
	if colonCount >= 2 {
		return true
	}

	// One colon, not in the drive letter position, is likely a separator.
	return firstColonIndex > 1
}

func isAlphaASCII(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// posixToWindowsPath converts common shell paths back into Windows drive paths.
// Supported inputs:
// - /c/some/path -> C:\some\path
// - /mnt/c/some/path -> C:\some\path
//
// If the input is not in a supported format, it returns ok=false.
func posixToWindowsPath(path string) (string, bool) {
	// WSL style: /mnt/c/...
	if strings.HasPrefix(path, "/mnt/") {
		// Minimum valid: "/mnt/c/" (len 7)
		if len(path) < 7 {
			return "", false
		}

		drive := path[5]
		if !isAlphaASCII(drive) || path[6] != '/' {
			return "", false
		}

		rest := path[7:]
		rest = strings.ReplaceAll(rest, "/", "\\")
		return strings.ToUpper(string(drive)) + ":\\" + rest, true
	}

	// Git Bash / MSYS style: /c/...
	if len(path) < 3 || path[0] != '/' {
		return "", false
	}

	drive := path[1]
	if !isAlphaASCII(drive) || path[2] != '/' {
		return "", false
	}

	rest := path[3:]
	rest = strings.ReplaceAll(rest, "/", "\\")
	return strings.ToUpper(string(drive)) + ":\\" + rest, true
}

// toShellPath converts a filesystem path into a shell-friendly posix-ish path.
//
// Behavior:
//   - If the input already starts with /, we assume it is already a shell path.
//   - If the input looks like a drive path (C:\... or C:/...), we convert it.
//     For WSL, that becomes /mnt/c/... and for other bash flavors it becomes /c/...
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
