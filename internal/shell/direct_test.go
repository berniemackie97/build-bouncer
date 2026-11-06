package shell

import (
	"slices"
	"testing"
)

func TestParseDirectInvocation(t *testing.T) {
	tests := []struct {
		name string
		input string
		executable string
		args []string
		matched bool
		fails bool
	}{
		{name: "bash quoted script", input: "bash -lc \"echo hello\"", executable: "bash", args: []string{"-lc", "echo hello"}, matched: true},
		{name: "quoted executable with spaces", input: "\"C:\\Program Files\\Git\\bin\\bash.exe\" -c 'echo hi'", executable: "C:\\Program Files\\Git\\bin\\bash.exe", args: []string{"-c", "echo hi"}, matched: true},
		{name: "single quoted script", input: "sh -c 'printf \"%s\" \"hello world\"'", executable: "sh", args: []string{"-c", "printf \"%s\" \"hello world\""}, matched: true},
		{name: "escaped double quote", input: "bash -c \"printf 'a\\\"b'\"", executable: "bash", args: []string{"-c", "printf 'a\"b'"}, matched: true},
		{name: "literal backslash", input: "bash -c \"printf '\\n'\"", executable: "bash", args: []string{"-c", "printf '\\n'"}, matched: true},
		{name: "powershell path", input: "\"C:\\Program Files\\PowerShell\\7\\pwsh.exe\" -NoProfile -Command \"Write-Host ok\"", executable: "C:\\Program Files\\PowerShell\\7\\pwsh.exe", args: []string{"-NoProfile", "-Command", "Write-Host ok"}, matched: true},
		{name: "powershell unquoted script", input: "pwsh -Command Write-Host \"hello world\"", executable: "pwsh", args: []string{"-Command", "Write-Host \"hello world\""}, matched: true},
		{name: "ordinary check", input: "go test ./..."},
		{name: "unrecognized flag", input: "bash -x script.sh"},
		{name: "missing script", input: "sh -c", matched: true, fails: true},
		{name: "unquoted script", input: "bash -lc echo", matched: true, fails: true},
		{name: "unterminated path", input: "\"C:\\Program Files\\Git\\bin\\bash.exe -c \"echo\"", matched: true, fails: true},
		{name: "unterminated script", input: "sh -c 'echo hi", matched: true, fails: true},
		{name: "embedded unmatched quote", input: "bash -c \"echo \"bad\"\"", matched: true, fails: true},
		{name: "empty script", input: "bash -c \"\"", matched: true, fails: true},
		{name: "powershell unterminated script", input: "pwsh -Command \"echo", matched: true, fails: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executable, args, matched, err := ParseDirectInvocation(tc.input)
			if matched != tc.matched || (err != nil) != tc.fails {
				t.Fatalf("ParseDirectInvocation(%q) = %q, %q, %t, %v", tc.input, executable, args, matched, err)
			}
			if !tc.fails && (executable != tc.executable || !slices.Equal(args, tc.args)) {
				t.Fatalf("ParseDirectInvocation(%q) = %q, %q; want %q, %q", tc.input, executable, args, tc.executable, tc.args)
			}
		})
	}
}
