package runner

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

func TestParallelVerboseChecksUseCallerOutput(t *testing.T) {
	var output bytes.Buffer
	checks := make([]config.Check, 16)
	for i := range checks {
		checks[i] = config.Check{Name: fmt.Sprintf("check-%02d", i), Run: "echo ready"}
	}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: checks}, Options{
		Verbose:     true,
		Output:      &output,
		MaxParallel: 8,
	})
	if err != nil || len(report.Failures) != 0 || len(report.Canceled) != 0 {
		t.Fatalf("parallel checks failed: %+v %v", report, err)
	}
	written := output.String()
	for _, check := range checks {
		if strings.Count(written, "==> "+check.Name+"\n") != 1 ||
			strings.Count(written, "OK "+check.Name+"\n") != 1 {
			t.Fatalf("missing or interleaved progress for %s in %q", check.Name, written)
		}
	}
	if strings.Count(written, "ready\n") != len(checks) {
		t.Fatalf("concurrent check output lost: %q", written)
	}
}

func TestSerializedWriterPreservesConcurrentMessages(t *testing.T) {
	var output bytes.Buffer
	writer := &serializedWriter{writer: &output}
	var group sync.WaitGroup
	for worker := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := range 100 {
				if _, err := fmt.Fprintf(writer, "worker-%d:%d\n", worker, iteration); err != nil {
					t.Errorf("writer: %v", err)
					return
				}
			}
		}()
	}
	group.Wait()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 800 {
		t.Fatalf("lost concurrent output: got %d lines", len(lines))
	}
	for _, line := range lines {
		var worker, iteration int
		if n, err := fmt.Sscanf(line, "worker-%d:%d", &worker, &iteration); n != 2 || err != nil {
			t.Fatalf("interleaved output line %q: %v", line, err)
		}
	}
}
