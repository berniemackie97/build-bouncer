package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/cli"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestCheckReportsOptionalInsultHistoryErrorsWithoutChangingEnforcement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		level   string
		allowed bool
		want    int
	}{
		{name: "strict blocks failed check", level: "strict", want: exitRunFailed},
		{name: "lax permits declared check failure", level: "lax", allowed: true, want: exitOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := withTempRepo(t)
			cfg := &config.Config{
				Checks:     []config.Check{{Name: "tests", Run: "exit 7", AllowFailure: tc.allowed}},
				Protection: config.Protection{Level: tc.level},
				Insults:    config.Insults{File: ".buildbouncer/insults.json", Locale: "en", Mode: "snarky"},
			}
			if err := config.Save(config.DefaultConfigPath(root), cfg); err != nil {
				t.Fatal(err)
			}
			packPath := filepath.Join(root, ".buildbouncer", "insults.json")
			if err := os.WriteFile(packPath, []byte(`{"templates":[{"id":"test","categories":["tests"],"text":"Fix {check}"}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			historyPath := filepath.Join(root, ".git", "build-bouncer", "insults_state.json")
			if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
				t.Fatal(err)
			}
			corrupt := []byte("{broken")
			if err := os.WriteFile(historyPath, corrupt, 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			got := runCheck(nil, cli.Context{Stdout: &stdout, Stderr: &stderr})
			if got != tc.want || !strings.Contains(stderr.String(), "Failure message state:") ||
				!strings.Contains(stderr.String(), "parse failure-message history") ||
				!strings.Contains(stderr.String(), "Push blocked") {
				t.Fatalf("incorrect policy or missing state diagnostic: code=%d want=%d stderr=%q stdout=%q", got, tc.want, stderr.String(), stdout.String())
			}
			after, err := os.ReadFile(historyPath)
			if err != nil || !bytes.Equal(after, corrupt) {
				t.Fatalf("corrupt cosmetic history was overwritten: %q %v", after, err)
			}
		})
	}
}
