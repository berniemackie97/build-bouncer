package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type Spinner struct {
	w      io.Writer
	frames []string

	mu      sync.Mutex
	message string

	stopCh chan struct{}
	doneCh chan struct{}
}

func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{
		w:      w,
		frames: []string{"|", "/", "-", "\\"},
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (s *Spinner) Start(initial string) {
	s.SetMessage(initial)

	go func() {
		defer close(s.doneCh)

		t := time.NewTicker(120 * time.Millisecond)
		defer t.Stop()

		i := 0
		for {
			select {
			case <-s.stopCh:
				return
			case <-t.C:
				msg := s.getMessage()
				if strings.TrimSpace(msg) == "" {
					msg = "..."
				}
				fmt.Fprintf(s.w, "\r%s %s", msg, s.frames[i%len(s.frames)])
				i++
			}
		}
	}()
}

func (s *Spinner) SetMessage(m string) {
	s.mu.Lock()
	s.message = strings.TrimSpace(m)
	s.mu.Unlock()
}

func (s *Spinner) Stop() {
	close(s.stopCh)
	<-s.doneCh

	// Clear line
	fmt.Fprint(s.w, "\r")
	fmt.Fprint(s.w, strings.Repeat(" ", 120))
	fmt.Fprint(s.w, "\r")
}

func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (s *Spinner) getMessage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.message
}
