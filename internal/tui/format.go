package tui

import (
	"fmt"
	"runtime"
	"strings"
)

// ANSI color codes
const (
	// Reset
	Reset = "\033[0m"

	// Regular colors
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bold colors
	BoldBlack   = "\033[1;30m"
	BoldRed     = "\033[1;31m"
	BoldGreen   = "\033[1;32m"
	BoldYellow  = "\033[1;33m"
	BoldBlue    = "\033[1;34m"
	BoldMagenta = "\033[1;35m"
	BoldCyan    = "\033[1;36m"
	BoldWhite   = "\033[1;37m"

	// Dim/faint colors
	DimWhite = "\033[2;37m"
	DimGray  = "\033[2;90m"
)

// Box drawing characters
const (
	BoxTopLeft     = "┌"
	BoxTopRight    = "┐"
	BoxBottomLeft  = "└"
	BoxBottomRight = "┘"
	BoxHorizontal  = "─"
	BoxVertical    = "│"
	BoxTeeRight    = "├"
	BoxTeeLeft     = "┤"
)

var colorsEnabled = true

// DisableColors turns off ANSI color output
func DisableColors() {
	colorsEnabled = false
}

// EnableColors turns on ANSI color output (default)
func EnableColors() {
	colorsEnabled = true
}

// Colorize wraps text in ANSI color codes if colors are enabled
func Colorize(color, text string) string {
	if !colorsEnabled || runtime.GOOS == "windows" && !isWindowsTerminalSupported() {
		return text
	}
	return color + text + Reset
}

// Error formats text in red
func Error(text string) string {
	return Colorize(BoldRed, text)
}

// Success formats text in green
func Success(text string) string {
	return Colorize(BoldGreen, text)
}

// Warning formats text in yellow
func Warning(text string) string {
	return Colorize(BoldYellow, text)
}

// Info formats text in cyan
func Info(text string) string {
	return Colorize(Cyan, text)
}

// Dim formats text in dimmed gray
func Dim(text string) string {
	return Colorize(DimGray, text)
}

// Bold formats text in bold white
func Bold(text string) string {
	return Colorize(BoldWhite, text)
}

// DrawBox creates a bordered box around content
func DrawBox(title string, content []string, width int) string {
	if width < 20 {
		width = 60
	}

	var b strings.Builder

	// Top border
	if title != "" {
		titleText := " " + title + " "
		padding := width - len(titleText) - 2
		if padding < 0 {
			padding = 0
		}
		leftPad := padding / 2
		rightPad := padding - leftPad

		b.WriteString(BoxTopLeft)
		b.WriteString(strings.Repeat(BoxHorizontal, leftPad))
		b.WriteString(titleText)
		b.WriteString(strings.Repeat(BoxHorizontal, rightPad))
		b.WriteString(BoxTopRight)
		b.WriteString("\n")
	} else {
		b.WriteString(BoxTopLeft)
		b.WriteString(strings.Repeat(BoxHorizontal, width-2))
		b.WriteString(BoxTopRight)
		b.WriteString("\n")
	}

	// Content lines
	for _, line := range content {
		lineLen := len(line)
		padding := width - lineLen - 4
		if padding < 0 {
			// Truncate line if too long
			line = line[:width-7] + "..."
			padding = 0
		}

		b.WriteString(BoxVertical)
		b.WriteString(" ")
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", padding))
		b.WriteString(" ")
		b.WriteString(BoxVertical)
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString(BoxBottomLeft)
	b.WriteString(strings.Repeat(BoxHorizontal, width-2))
	b.WriteString(BoxBottomRight)

	return b.String()
}

// Section creates a section header with a line underneath
func Section(title string) string {
	line := strings.Repeat("─", len(title))
	return fmt.Sprintf("%s\n%s", Bold(title), Dim(line))
}

// Bullet creates a bulleted list item
func Bullet(text string) string {
	return Dim("  • ") + text
}

// Check creates a checkmark bullet
func Check(text string) string {
	return Success("  ✓ ") + text
}

// Cross creates a cross/X bullet
func Cross(text string) string {
	return Error("  ✗ ") + text
}

// Arrow creates an arrow bullet
func Arrow(text string) string {
	return Info("  → ") + text
}

// isWindowsTerminalSupported checks if Windows terminal supports ANSI
func isWindowsTerminalSupported() bool {
	// Windows Terminal, VS Code terminal, and Git Bash support ANSI
	term := strings.ToLower(fmt.Sprintf("%s", []byte{}))
	return strings.Contains(term, "xterm") ||
		strings.Contains(term, "vscode") ||
		strings.Contains(term, "git")
}
