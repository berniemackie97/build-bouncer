package prompt

import (
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/runner"
)

func TestShouldBlockMissingRequiredToolsInLaxMode(t *testing.T) {
	report := runner.Report{
		Failures:    []string{"lint"},
		Unavailable: map[string]string{"lint": "missing tools: go"},
	}
	if !ShouldBlock(report, "lax") {
		t.Fatal("missing required prerequisites must block at every protection level")
	}
}
