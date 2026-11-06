package shell

import "testing"

func TestParseExecutable(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		want     string
		wantFail bool
	}{
		{name: "empty"},
		{name: "simple executable", spec: "pwsh", want: "pwsh"},
		{name: "POSIX path", spec: "/usr/bin/bash", want: "/usr/bin/bash"},
		{name: "quoted POSIX path with spaces", spec: "\"/tmp/shell binaries/sh\"", want: "/tmp/shell binaries/sh"},
		{name: "quoted Windows path", spec: "\"C:\\Program Files\\Git\\bin\\bash.exe\"", want: "C:\\Program Files\\Git\\bin\\bash.exe"},
		{name: "unquoted Windows path", spec: "C:\\Program Files\\Git\\bin\\bash.exe", want: "C:\\Program Files\\Git\\bin\\bash.exe"},
		{name: "single quoted path", spec: "'C:\\Program Files\\Git\\bin\\bash.exe'", want: "C:\\Program Files\\Git\\bin\\bash.exe"},
		{name: "unquoted args", spec: "bash -lc", wantFail: true},
		{name: "quoted path with args", spec: "\"C:\\Program Files\\Git\\bin\\bash.exe\" -lc", wantFail: true},
		{name: "path followed by args", spec: "C:\\Program Files\\Git\\bin\\bash.exe -lc", wantFail: true},
		{name: "ambiguous POSIX path", spec: "/tmp/shell binaries/sh", wantFail: true},
		{name: "unterminated quote", spec: "\"/tmp/shell", wantFail: true},
		{name: "empty quoted path", spec: "\"\"", wantFail: true},
		{name: "invalid executable", spec: "-lc", wantFail: true},
		{name: "newline", spec: "bash\nother", wantFail: true},
		{name: "tab", spec: "bash\tother", wantFail: true},
		{name: "NUL", spec: "bash\x00other", wantFail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseExecutable(tc.spec)
			if tc.wantFail {
				if err == nil {
					t.Fatalf("expected invalid shell spec %q to fail, got %q", tc.spec, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ParseExecutable(%q) = %q, %v; want %q", tc.spec, got, err, tc.want)
			}
		})
	}
}

func TestBaseNameAcrossPlatforms(t *testing.T) {
	for _, tc := range []struct{ spec, want string }{
		{spec: "C:\\Tools\\bin\\bash.exe", want: "bash.exe"},
		{spec: "/usr/local/bin/pwsh", want: "pwsh"},
		{spec: "cmd.exe", want: "cmd.exe"},
	} {
		if got := BaseName(tc.spec); got != tc.want {
			t.Fatalf("BaseName(%q) = %q, want %q", tc.spec, got, tc.want)
		}
	}
}
