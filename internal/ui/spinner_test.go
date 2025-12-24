package ui_test

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/berniemackie97/build-bouncer/internal/ui"
)

type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (writer *lockedBuffer) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.buffer.Write(data)
}

func (writer *lockedBuffer) String() string {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.buffer.String()
}

func (writer *lockedBuffer) Len() int {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.buffer.Len()
}

func TestSpinner_Start_WritesOutputAndStopStopsWriting(t *testing.T) {
	output := &lockedBuffer{}
	spinner := ui.NewSpinner(output)

	spinner.Start("Running checks")
	time.Sleep(200 * time.Millisecond)

	spinner.Stop()

	writtenAfterStopLength := output.Len()
	time.Sleep(200 * time.Millisecond)

	if output.Len() != writtenAfterStopLength {
		t.Fatalf("spinner kept writing after Stop, length changed from %d to %d", writtenAfterStopLength, output.Len())
	}

	if !strings.Contains(output.String(), "Running checks") {
		t.Fatalf("expected spinner output to include message, got %q", output.String())
	}
}

func TestSpinner_Start_IsIdempotent_UpdatesMessageInsteadOfStartingAnotherLoop(t *testing.T) {
	output := &lockedBuffer{}
	spinner := ui.NewSpinner(output)

	spinner.Start("First")
	time.Sleep(150 * time.Millisecond)

	// This should not start a second goroutine. It should just update the message.
	spinner.Start("Second")
	time.Sleep(200 * time.Millisecond)

	spinner.Stop()

	if !strings.Contains(output.String(), "Second") {
		t.Fatalf("expected spinner output to include updated message, got %q", output.String())
	}

	writtenAfterStopLength := output.Len()
	time.Sleep(200 * time.Millisecond)

	if output.Len() != writtenAfterStopLength {
		t.Fatalf("spinner kept writing after Stop, length changed from %d to %d", writtenAfterStopLength, output.Len())
	}
}

func TestSpinner_SetMessage_UpdatesWhileRunning(t *testing.T) {
	output := &lockedBuffer{}
	spinner := ui.NewSpinner(output)

	spinner.Start("Initial")
	time.Sleep(150 * time.Millisecond)

	spinner.SetMessage("Updated")
	time.Sleep(200 * time.Millisecond)

	spinner.Stop()

	if !strings.Contains(output.String(), "Updated") {
		t.Fatalf("expected spinner output to include updated message, got %q", output.String())
	}
}

func TestSpinner_Stop_WhenNotRunning_DoesNothing(t *testing.T) {
	output := &lockedBuffer{}
	spinner := ui.NewSpinner(output)

	spinner.Stop()

	if output.Len() != 0 {
		t.Fatalf("expected no output when Stop is called while not running, got %q", output.String())
	}
}

func TestSpinner_Stop_IsIdempotent(t *testing.T) {
	output := &lockedBuffer{}
	spinner := ui.NewSpinner(output)

	spinner.Start("Run")
	time.Sleep(150 * time.Millisecond)

	spinner.Stop()
	lengthAfterFirstStop := output.Len()

	// Second stop should not panic and should not write anything.
	spinner.Stop()
	lengthAfterSecondStop := output.Len()

	if lengthAfterSecondStop != lengthAfterFirstStop {
		t.Fatalf("expected second Stop to be a no op, length changed from %d to %d", lengthAfterFirstStop, lengthAfterSecondStop)
	}
}

func TestIsTerminal_ReturnsFalseForNormalFile(t *testing.T) {
	tempFile, err := os.CreateTemp("", "build-bouncer-ui-terminal-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	if ui.IsTerminal(tempFile) {
		t.Fatal("expected temp file to not be a terminal")
	}
}

func TestIsTerminal_ReturnsFalseWhenStatFails(t *testing.T) {
	tempFile, err := os.CreateTemp("", "build-bouncer-ui-terminal-test-closed-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	fileName := tempFile.Name()
	_ = tempFile.Close()
	_ = os.Remove(fileName)

	if ui.IsTerminal(tempFile) {
		t.Fatal("expected closed file to not be a terminal")
	}
}
