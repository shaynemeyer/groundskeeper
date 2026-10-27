// Package notify posts the macOS notification at the end of a scheduled
// scan. Text is passed to osascript as arguments, never as script source,
// so nothing in a message can run as AppleScript.
package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// Title is the notification title.
const Title = "groundskeeper"

// script reads its text from argv instead of having it spliced in.
var script = []string{
	"-e", "on run argv",
	"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
	"-e", "end run",
}

// Runner runs a command; tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Summary is what the notification reports.
type Summary struct {
	Free        int64
	Reclaimable int64
	Problems    int // scanners that failed or timed out
	NewSkipped  int // paths skipped now that weren't skipped last time
}

// Message is the notification text, in Rook's voice: the useful number
// first, problems stated plainly.
func Message(s Summary) string {
	msg := fmt.Sprintf("Rook's report is ready: %s free, about %s to tidy.", ui.Bytes(s.Free), ui.Bytes(s.Reclaimable))
	var problems []string
	if s.Problems > 0 {
		problems = append(problems, plural(s.Problems, "scanner", "scanners")+" had problems")
	}
	if s.NewSkipped > 0 {
		problems = append(problems, plural(s.NewSkipped, "new path", "new paths")+" couldn't be read")
	}
	if len(problems) > 0 {
		msg += " " + strings.Join(problems, ", ") + "; see the report."
	}
	return msg
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Send posts msg as a notification.
func Send(ctx context.Context, run Runner, msg string) error {
	_, err := run(ctx, "osascript", append(script, msg, Title)...)
	return err
}
