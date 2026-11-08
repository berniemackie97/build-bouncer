package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
	written := strings.ReplaceAll(output.String(), "\r\n", "\n")
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

func TestSerializedWriterPreservesFirstOutputError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "write failure", writer: failingWriter{}, want: os.ErrPermission},
		{name: "short write", writer: shortWriter{}, want: io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var canceled int
			writer := &serializedWriter{writer: tc.writer, cancel: func() { canceled++ }}
			_, err := writer.Write([]byte("progress"))
			if !errors.Is(err, tc.want) || !errors.Is(writer.Err(), tc.want) || canceled != 1 {
				t.Fatalf("missing first output failure: err=%v stored=%v canceled=%d", err, writer.Err(), canceled)
			}
			_, err = writer.Write([]byte("again"))
			if !errors.Is(err, tc.want) || canceled != 1 {
				t.Fatalf("first failure was lost: err=%v canceled=%d", err, canceled)
			}
		})
	}
}

type failAfterOutputWrites struct {
	count  int
	failAt int
}

func (w *failAfterOutputWrites) Write(p []byte) (int, error) {
	w.count++
	if w.count >= w.failAt {
		return 0, os.ErrPermission
	}
	return len(p), nil
}

func TestRunAllReportFailsOnVerboseOutputErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "failed start message", writer: failingWriter{}, want: os.ErrPermission},
		{name: "short start message", writer: shortWriter{}, want: io.ErrShortWrite},
		{name: "failed success message", writer: &failAfterOutputWrites{failAt: 2}, want: os.ErrPermission},
		{name: "failed subprocess output", writer: &failAfterOutputWrites{failAt: 2}, want: os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "exit 0"
			if tc.name == "failed subprocess output" {
				command = "echo streamed"
			}
			report, err := RunAllReport(t.TempDir(), &config.Config{Checks: []config.Check{{Name: "check", Run: command}}}, Options{
				Verbose: true, Output: tc.writer,
			})
			if !errors.Is(err, ErrOutputWrite) || !errors.Is(err, tc.want) {
				t.Fatalf("output failure produced successful report: report=%+v err=%v", report, err)
			}
		})
	}
}

func TestParallelRunnerStopsOnOutputSinkFailure(t *testing.T) {
	checks := make([]config.Check, 16)
	for i := range checks {
		checks[i] = config.Check{Name: fmt.Sprintf("check-%02d", i), Run: "echo must-not-run"}
	}
	report, err := RunAllReport(t.TempDir(), &config.Config{Checks: checks}, Options{
		Verbose: true, MaxParallel: 8, Output: failingWriter{},
	})
	if !errors.Is(err, ErrOutputWrite) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("parallel output failure accepted: %+v %v", report, err)
	}
	if len(report.Canceled) != len(checks) || len(report.Failures) != 0 {
		t.Fatalf("output failure did not stop dispatch and running checks: %+v", report)
	}
}
