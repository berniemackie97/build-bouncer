package hooks

import "testing"

func TestOwnedPrePushHookRequiresHeader(t *testing.T) {
	tests := []struct {
		name string
		body string
		ours bool
	}{
		{name: "current header", body: "#!/bin/sh\n# build-bouncer pre-push hook v3\nexit 0\n", ours: true},
		{name: "previous version", body: "#!/bin/sh\n# build-bouncer pre-push hook v1\nexit 0\n", ours: true},
		{name: "previous version 2", body: "#!/bin/sh\n# build-bouncer pre-push hook v2\nexit 0\n", ours: true},
		{name: "CRLF header", body: "#!/bin/sh\r\n# build-bouncer pre-push hook v3\r\nexit 0\r\n", ours: true},
		{name: "marker in body", body: "#!/bin/sh\necho foreign\n# build-bouncer pre-push hook v3\n", ours: false},
		{name: "marker in arbitrary text", body: "echo '# build-bouncer pre-push hook v3'\n", ours: false},
		{name: "marker without shebang", body: "# build-bouncer pre-push hook v3\nexit 0\n", ours: false},
		{name: "wrong shebang", body: "#!/bin/bash\n# build-bouncer pre-push hook v3\nexit 0\n", ours: false},
		{name: "unsupported version", body: "#!/bin/sh\n# build-bouncer pre-push hook v4\nexit 0\n", ours: false},
		{name: "extra marker text", body: "#!/bin/sh\n# build-bouncer pre-push hook v3 (foreign)\nexit 0\n", ours: false},
		{name: "header only", body: "#!/bin/sh\n# build-bouncer pre-push hook v3", ours: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownedPrePushHook([]byte(tc.body)); got != tc.ours {
				t.Fatalf("ownedPrePushHook() = %v, want %v", got, tc.ours)
			}
		})
	}
}
