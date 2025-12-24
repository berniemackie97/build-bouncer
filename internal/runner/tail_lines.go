package runner

import "strings"

// TailLines returns the last n lines from a block of text after dropping any trailing blank lines.
// A blank line is one where TrimSpace(line) is empty.
// Newlines are normalized to LF first.
func TailLines(text string, n int) string {
	if n <= 0 {
		return ""
	}

	normalized := normalizeNewlinesToLF(text)
	if normalized == "" {
		return ""
	}

	// Step 1: drop trailing blank lines without splitting the whole string into a slice.
	// We repeatedly examine the last line, and if it is blank, we remove it and its newline.
	end := len(normalized)
	for end > 0 {
		lastNL := strings.LastIndexByte(normalized[:end], '\n')
		lineStart := lastNL + 1
		lastLine := normalized[lineStart:end]

		if strings.TrimSpace(lastLine) != "" {
			break
		}

		// No newline means the entire string is one blank line.
		if lastNL == -1 {
			end = 0
			break
		}

		// Remove the blank line, including the newline that ends it.
		end = lastNL
	}

	if end == 0 {
		return ""
	}

	// Step 2: find the start index of the last n lines.
	// We walk backwards counting newline separators.
	from := 0
	pos := end
	newlinesFound := 0

	for newlinesFound < n && pos > 0 {
		idx := strings.LastIndexByte(normalized[:pos], '\n')
		if idx == -1 {
			from = 0
			break
		}

		from = idx + 1
		pos = idx
		newlinesFound++
	}

	// If we ran out of string but still wanted more lines, include from the beginning.
	// This preserves cases where the first included line is an empty line.
	if newlinesFound < n && pos == 0 {
		from = 0
	}

	return normalized[from:end]
}

// normalizeNewlinesToLF converts CRLF and CR newlines into LF.
// This keeps the rest of the code simple and consistent.
func normalizeNewlinesToLF(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return normalized
}
