package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer lets the spinner goroutine and the test share a buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSpinnerShowsLabelAndClearsOnStop(t *testing.T) {
	var out syncBuffer
	s := startSpinner(&out, time.Millisecond)
	s.Set("Reading iPhone")
	time.Sleep(20 * time.Millisecond)
	s.Set("Scanning Caches")
	time.Sleep(20 * time.Millisecond)
	s.Stop()

	got := out.String()
	for _, want := range []string{"Reading iPhone...", "Scanning Caches..."} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if !strings.HasSuffix(got, clearLine) {
		t.Errorf("output doesn't end by clearing the line: %q", got[max(0, len(got)-40):])
	}
}

func TestSpinnerSilentWhenPlain(t *testing.T) {
	var out bytes.Buffer
	s := StartSpinner(&out)
	s.Set("Reading iPhone")
	s.Stop()
	if out.Len() != 0 {
		t.Errorf("plain output got %q", out.String())
	}
}

func TestSpinnerStopTwice(t *testing.T) {
	s := startSpinner(&syncBuffer{}, time.Millisecond)
	s.Stop()
	s.Stop()
	(&Spinner{}).Stop()
}
