// Package metrics describes what each run did. Metrics are stored in
// history snapshots and shown in the report and by the stats command;
// they never leave the Mac.
package metrics

import (
	"strconv"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// SlowScanner is how long a scanner can take before it is highlighted.
const SlowScanner = time.Minute

// Run describes one scan.
type Run struct {
	Start       time.Time  `json:"start"`
	DurationMS  int64      `json:"durationMs"`
	Command     string     `json:"command"`
	Interactive bool       `json:"interactive"` // false for launchd runs and pipes
	Scanners    []Scanner  `json:"scanners"`
	Size        size.Stats `json:"size"`
}

// Scanner describes one scanner within a run.
type Scanner struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"durationMs"`
	Items      int    `json:"items"`
	Bytes      int64  `json:"bytes"`
	Skipped    int    `json:"skipped"`
	Timeouts   int    `json:"timeouts"`
	Error      string `json:"error,omitempty"` // error kind, never a message
}

// Slow reports whether the scanner should be highlighted.
func (s Scanner) Slow() bool {
	return s.DurationMS >= SlowScanner.Milliseconds() || s.Timeouts > 0
}

// Status is "failed (kind)", "N timed out", "slow" or "ok".
func (s Scanner) Status() string {
	switch {
	case s.Error != "":
		return "failed (" + s.Error + ")"
	case s.Timeouts > 0:
		return strconv.Itoa(s.Timeouts) + " timed out"
	case s.Slow():
		return "slow"
	}
	return "ok"
}

// Errors counts scanners that failed.
func (r Run) Errors() int {
	n := 0
	for _, s := range r.Scanners {
		if s.Error != "" {
			n++
		}
	}
	return n
}

// Clean describes one clean command run.
type Clean struct {
	Time       time.Time `json:"time"`
	Command    string    `json:"command"`
	DryRun     bool      `json:"dryRun"`
	Moved      int       `json:"moved"`
	MovedBytes int64     `json:"movedBytes"`
	Refused    int       `json:"refused"`
	Failed     int       `json:"failed"`
}
