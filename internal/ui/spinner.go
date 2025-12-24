// Package ui contains small terminal UI helpers used by build bouncer.
// This stuff is cosmetic. It should never be able to crash a run.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const spinnerTickDuration = 120 * time.Millisecond

type Spinner struct {
	outputWriter       io.Writer
	frameSequence      []string
	mutex              sync.Mutex
	currentMessage     string
	isRunning          bool
	hasStopBeenRequest bool
	stopSignal         chan struct{}
	doneSignal         chan struct{}
	lastRenderedLength int
}

func NewSpinner(writer io.Writer) *Spinner {
	if writer == nil {
		writer = io.Discard
	}

	return &Spinner{
		outputWriter:  writer,
		frameSequence: []string{"|", "/", "-", "\\"},
		stopSignal:    make(chan struct{}),
		doneSignal:    make(chan struct{}),
	}
}

// Start begins the spinner loop.
// If Start is called while already running, it updates the message and returns.
// This prevents multiple goroutines from fighting over the same terminal line.
func (spinner *Spinner) Start(initialMessage string) {
	trimmedMessage := strings.TrimSpace(initialMessage)

	spinner.mutex.Lock()
	if spinner.isRunning {
		spinner.mutex.Unlock()
		// SetMessage will relock the mutex
		spinner.SetMessage(trimmedMessage)
		return
	}

	spinner.currentMessage = trimmedMessage
	spinner.isRunning = true
	spinner.hasStopBeenRequest = false

	stopSignal := spinner.stopSignal
	doneSignal := spinner.doneSignal
	spinner.mutex.Unlock()

	go func() {
		defer close(doneSignal)

		ticker := time.NewTicker(spinnerTickDuration)
		defer ticker.Stop()

		frameIndex := 0

		for {
			select {
			case <-stopSignal:
				return
			case <-ticker.C:
				messageText := spinner.getMessage()
				if strings.TrimSpace(messageText) == "" {
					messageText = "..."
				}

				frameText := spinner.frameSequence[frameIndex%len(spinner.frameSequence)]
				frameIndex++

				renderedLine := fmt.Sprintf("\r%s %s", messageText, frameText)

				spinner.mutex.Lock()
				previousLineLength := spinner.lastRenderedLength

				renderedVisibleLength := len(renderedLine) - 1
				if renderedVisibleLength < previousLineLength {
					paddingLength := previousLineLength - renderedVisibleLength
					renderedLine += strings.Repeat(" ", paddingLength)
					renderedVisibleLength = previousLineLength
				}

				spinner.lastRenderedLength = renderedVisibleLength
				spinner.mutex.Unlock()

				_, _ = fmt.Fprint(spinner.outputWriter, renderedLine)

			}
		}
	}()
}

// SetMessage updates the spinner message.
// Safe to call while the spinner is running.
func (spinner *Spinner) SetMessage(message string) {
	trimmedMessage := strings.TrimSpace(message)

	spinner.mutex.Lock()
	spinner.currentMessage = trimmedMessage
	spinner.mutex.Unlock()
}

// Stop shuts down the spinner safely.
// Calling Stop when it is not running does nothing
// Calling Stop multiple times does not panic.
func (spinner *Spinner) Stop() {
	spinner.mutex.Lock()
	if !spinner.isRunning {
		spinner.mutex.Unlock()
		return
	}

	if !spinner.hasStopBeenRequest {
		close(spinner.stopSignal)
		spinner.hasStopBeenRequest = true
	}

	doneSignal := spinner.doneSignal
	spinner.mutex.Unlock()

	<-doneSignal

	spinner.mutex.Lock()
	clearLength := spinner.lastRenderedLength

	spinner.lastRenderedLength = 0
	spinner.isRunning = false
	spinner.hasStopBeenRequest = false

	spinner.stopSignal = make(chan struct{})
	spinner.doneSignal = make(chan struct{})
	spinner.mutex.Unlock()

	// Clear exactly what we drew last time.
	_, _ = fmt.Fprint(spinner.outputWriter, "\r")
	if clearLength > 0 {
		_, _ = fmt.Fprint(spinner.outputWriter, strings.Repeat(" ", clearLength))
		_, _ = fmt.Fprint(spinner.outputWriter, "\r")
	}
}

func IsTerminal(file *os.File) bool {
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func (spinner *Spinner) getMessage() string {
	spinner.mutex.Lock()
	defer spinner.mutex.Unlock()
	return spinner.currentMessage
}
