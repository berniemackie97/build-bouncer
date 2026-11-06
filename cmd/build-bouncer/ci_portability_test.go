package main

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/ci"
	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestCheckedInConfigurationMatchesWorkflow(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	cfg, err := config.Load(filepath.Join(root, ".buildbouncer", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ci.ChecksFromGitHubActions(root)
	if err != nil {
		t.Fatal(err)
	}
	generated := stampGeneratedChecks(imported, "ci")
	var configured []config.Check
	for _, check := range cfg.Checks {
		if isCICheck(check) {
			configured = append(configured, check)
		}
		if check.Source == "template:go" && check.Shell != "" {
			t.Fatalf("template shell is tied to a single platform: %+v", check)
		}
	}
	if !reflect.DeepEqual(configured, generated) {
		t.Fatalf("checked-in CI checks differ from imported workflows: configured=%+v generated=%+v", configured, generated)
	}
}
