package shell

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseDirectInvocation recognizes explicit POSIX and PowerShell script invocations.
// Malformed recognized invocations return errors rather than falling back to another shell.
func ParseDirectInvocation(command string) (executable string, args []string, matched bool, err error) {
	input := strings.TrimSpace(command)
	executable, remaining, valid := firstCommandToken(input)
	if !valid {
		if strings.HasPrefix(input, "\"") || strings.HasPrefix(input, "'") {
			return "", nil, true, fmt.Errorf("invalid quoted command executable")
		}
		return "", nil, false, nil
	}

	switch strings.ToLower(BaseName(executable)) {
	case "bash", "bash.exe", "sh", "sh.exe":
		flag, rest, valid := firstCommandToken(remaining)
		if !valid || flag != "-c" && flag != "-lc" {
			return "", nil, false, nil
		}
		script, valid := quotedCommand(rest)
		if !valid || strings.TrimSpace(script) == "" {
			return "", nil, true, fmt.Errorf("%s %s requires a nonempty, properly quoted script", executable, flag)
		}
		return executable, []string{flag, script}, true, nil
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		var options []string
		for remaining != "" {
			flag, rest, valid := firstCommandToken(remaining)
			if !valid {
				return "", nil, true, fmt.Errorf("invalid PowerShell invocation")
			}
			if strings.EqualFold(flag, "-Command") || strings.EqualFold(flag, "-c") {
				script := strings.TrimSpace(rest)
				if script == "" {
					return "", nil, true, fmt.Errorf("%s %s requires a nonempty script", executable, flag)
				}
				if script[0] == '"' || script[0] == '\'' {
					unquoted, valid := quotedCommand(script)
					if !valid {
						return "", nil, true, fmt.Errorf("%s %s has invalid script quoting", executable, flag)
					}
					script = unquoted
				}
				if strings.TrimSpace(script) == "" {
					return "", nil, true, fmt.Errorf("%s %s requires a nonempty script", executable, flag)
				}
				return executable, append(options, flag, script), true, nil
			}
			options = append(options, flag)
			remaining = rest
		}
	}
	return "", nil, false, nil
}

func firstCommandToken(text string) (token, remaining string, valid bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}
	if text[0] == '"' || text[0] == '\'' {
		closing := strings.IndexByte(text[1:], text[0])
		if closing < 0 {
			return "", "", false
		}
		after := text[closing+2:]
		if after != "" && !unicode.IsSpace(rune(after[0])) {
			return "", "", false
		}
		return text[1 : closing+1], strings.TrimSpace(after), true
	}
	end := strings.IndexFunc(text, unicode.IsSpace)
	if end < 0 {
		return text, "", true
	}
	return text[:end], strings.TrimSpace(text[end:]), true
}

func quotedCommand(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return "", false
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return "", false
	}
	var script strings.Builder
	for i := 1; i < len(text)-1; i++ {
		char := text[i]
		if quote == '"' && char == '\\' {
			if i+1 == len(text)-1 {
				return "", false
			}
			if next := text[i+1]; next == '"' || next == '\\' {
				script.WriteByte(next)
				i++
				continue
			}
		}
		if char == quote {
			return "", false
		}
		script.WriteByte(char)
	}
	return script.String(), true
}
