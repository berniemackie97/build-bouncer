package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/config"
)

type cancelOnCheckOutput struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (writer *cancelOnCheckOutput) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("cancel-ready")) {
		writer.once.Do(writer.cancel)
	}
	return len(p), nil
}

func TestCanceledCheckRetainsLogAndTail(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &config.Config{Checks: []config.Check{{
		Name: "canceled", Run: "echo cancel-ready && " + sleepCommand(5),
	}}}
	start := time.Now()
	report, err := RunAllReport(root, cfg, Options{
		Context: ctx, Verbose: true, MaxLogBytes: 3,
		Output: &cancelOnCheckOutput{cancel: cancel},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled suite returned %v; report=%+v", err, report)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("cancellation did not stop the running check promptly")
	}
	if len(report.Canceled) != 1 || report.Canceled[0] != "canceled" || len(report.Failures) != 0 {
		t.Fatalf("canceled check was misclassified: %+v", report)
	}
	if !strings.Contains(report.FailureTails["canceled"], "cancel-ready") {
		t.Fatalf("missing retained output tail: %+v", report.FailureTails)
	}
	path := report.LogFiles["canceled"]
	if !strings.HasSuffix(path, ".log") || !report.LogTruncated["canceled"] {
		t.Fatalf("canceled check lost log metadata: %+v", report)
	}
	contents, err := os.ReadFile(path)
	if err != nil || len(contents) != 3 {
		t.Fatalf("missing bounded canceled log: %q %v", contents, err)
	}
}
