package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// spinnerFrames are the braille dots Bubbles' spinner.Dot uses.
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// clearLine moves to the start of the line and erases it.
const clearLine = "\r\x1b[2K"

// Spinner animates one status line, such as the scanner that is running.
// It writes nothing when output is plain. The zero Spinner is silent.
type Spinner struct {
	w        io.Writer
	mu       sync.Mutex
	label    string
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// StartSpinner starts a spinner on w, or a silent one when w is plain.
func StartSpinner(w io.Writer) *Spinner {
	if Plain(w) {
		return &Spinner{}
	}
	return startSpinner(w, 100*time.Millisecond)
}

func startSpinner(w io.Writer, every time.Duration) *Spinner {
	s := &Spinner{w: w, stop: make(chan struct{}), done: make(chan struct{})}
	go s.run(every)
	return s
}

func (s *Spinner) run(every time.Duration) {
	defer close(s.done)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for i := 0; ; i++ {
		s.mu.Lock()
		if s.label != "" {
			fmt.Fprintf(s.w, "%s%s %s...", clearLine, spinnerFrames[i%len(spinnerFrames)], s.label)
		}
		s.mu.Unlock()
		select {
		case <-s.stop:
			fmt.Fprint(s.w, clearLine)
			return
		case <-tick.C:
		}
	}
}

// Set changes the status line's label.
func (s *Spinner) Set(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop erases the status line and waits for the spinner to finish. Calls
// after the first do nothing.
func (s *Spinner) Stop() {
	if s.stop == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
}
