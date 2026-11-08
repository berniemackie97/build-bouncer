package hooks

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestOwnedPrePushHookRequiresHeader(t *testing.T) {
	tests := []struct {
		name string
		body string
		ours bool
	}{
		{name: "current header", body: "#!/bin/sh\n# build-bouncer pre-push hook v4\nexit 0\n", ours: true},
		{name: "prior v3", body: "#!/bin/sh\n# build-bouncer pre-push hook v3\nexit 0\n", ours: true},
		{name: "previous version", body: "#!/bin/sh\n# build-bouncer pre-push hook v1\nexit 0\n", ours: true},
		{name: "previous version 2", body: "#!/bin/sh\n# build-bouncer pre-push hook v2\nexit 0\n", ours: true},
		{name: "CRLF header", body: "#!/bin/sh\r\n# build-bouncer pre-push hook v3\r\nexit 0\r\n", ours: true},
		{name: "marker in body", body: "#!/bin/sh\necho foreign\n# build-bouncer pre-push hook v3\n", ours: false},
		{name: "marker in arbitrary text", body: "echo '# build-bouncer pre-push hook v3'\n", ours: false},
		{name: "marker without shebang", body: "# build-bouncer pre-push hook v3\nexit 0\n", ours: false},
		{name: "wrong shebang", body: "#!/bin/bash\n# build-bouncer pre-push hook v3\nexit 0\n", ours: false},
		{name: "unsupported version", body: "#!/bin/sh\n# build-bouncer pre-push hook v5\nexit 0\n", ours: false},
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

func TestLegacyGeneratedV3ScriptSignature(t *testing.T) {
	for _, tc := range []struct {
		name   string
		copied bool
		sha256 string
	}{
		{name: "PATH only", copied: false, sha256: "be4addb367c5b19ef0489ed21bc10e161aa14c997adbdfa15c6a1f4e18489bfa"},
		{name: "copied executable", copied: true, sha256: "a9c2a737d38e3197bf6cf33b4731c46fee1b37c2fb871eda2ff20b33850584a6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(renderLegacyPrePushHook(tc.copied))
			if digest := fmt.Sprintf("%x", sha256.Sum256(body)); digest != tc.sha256 {
				t.Fatalf("legacy hook script changed; existing installations would no longer be recognized: %s", digest)
			}
			if !knownGeneratedPrePushHook(body) || generatedPrePushHook(body) {
				t.Fatal("v3 must be recognized for safe migration but must not be reported as current")
			}
		})
	}
}
