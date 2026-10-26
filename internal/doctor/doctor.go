// Package doctor checks what groundskeeper needs to work well and says how
// to fix anything that's missing. It only reads; it never changes settings.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
)

// Label is the LaunchAgent's launchd label.
const Label = "com.lot86.groundskeeper"

// staleSchedule is how old the last scheduled run can be before warning.
// The schedule runs twice a week, so a week without a run means it stopped.
const staleSchedule = 7 * 24 * time.Hour

// Statuses for a check.
const (
	OK   = "ok"
	Warn = "warn"
	Fail = "fail"
)

// Check is one result: what was checked, how it went, and how to fix it.
type Check struct {
	Name   string
	Status string
	Detail string
	Fix    string
}

// Runner runs a command; tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Env is what the checks look at.
type Env struct {
	Home      string
	OutputDir string // absolute
	Run       Runner
	Now       time.Time
	Scans     []history.Snapshot
}

// Run performs every check, in the order they're shown.
func Run(ctx context.Context, env Env) []Check {
	return []Check{
		installed(env),
		photosHelper(env),
		fullDiskAccess(env),
		schedule(ctx, env),
		lastScheduledRun(env),
		fileVault(ctx, env),
		outputFolder(env),
		logFiles(env),
	}
}

// PlistPath is where the LaunchAgent is installed.
func PlistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

func installed(env Env) Check {
	c := Check{Name: "Installed binary"}
	path := filepath.Join(env.Home, "bin", "groundskeeper")
	if _, err := os.Stat(path); err != nil {
		c.Status, c.Detail, c.Fix = Fail, path+" is missing", "Run make install in the groundskeeper repo."
		return c
	}
	c.Status, c.Detail = OK, path
	return c
}

// HelperPath is where make install puts photos-helper. Groundskeeper runs
// it by this absolute path, never through $PATH.
func HelperPath(home string) string {
	return filepath.Join(home, "bin", "photos-helper")
}

// photosHelper warns rather than fails: only photo scanning needs it.
func photosHelper(env Env) Check {
	c := Check{Name: "Photos helper"}
	path := HelperPath(env.Home)
	info, err := os.Stat(path)
	switch {
	case err != nil:
		c.Status, c.Detail = Warn, path+" is missing, so photo scanning won't work"
	case !info.Mode().IsRegular() || info.Mode().Perm()&0o100 == 0:
		c.Status, c.Detail = Warn, path+" isn't an executable file"
	default:
		c.Status, c.Detail = OK, path
		return c
	}
	c.Fix = "Run make install in the groundskeeper repo."
	return c
}

// fullDiskAccess reads ~/Library/Safari, which macOS protects. It reports
// on the process running doctor; the schedule needs the grant on the
// binary itself.
func fullDiskAccess(env Env) Check {
	c := Check{Name: "Full Disk Access"}
	_, err := os.ReadDir(filepath.Join(env.Home, "Library", "Safari"))
	switch {
	case err == nil, errors.Is(err, fs.ErrNotExist):
		c.Status, c.Detail = OK, "protected folders are readable"
	default:
		c.Status = Warn
		c.Detail = "protected folders such as Safari, Mail and some browser data are skipped"
		c.Fix = "System Settings > Privacy & Security > Full Disk Access: add your terminal app for runs you start, and " +
			filepath.Join(env.Home, "bin", "groundskeeper") + " for the schedule."
	}
	return c
}

func schedule(ctx context.Context, env Env) Check {
	c := Check{Name: "Schedule"}
	plist := PlistPath(env.Home)
	if _, err := os.Stat(plist); err != nil {
		c.Status, c.Detail, c.Fix = Fail, "LaunchAgent not installed", "Run make install in the groundskeeper repo."
		return c
	}
	if _, err := env.Run(ctx, "launchctl", "print", fmt.Sprintf("gui/%d/%s", os.Getuid(), Label)); err != nil {
		c.Status, c.Detail = Fail, "LaunchAgent installed but not loaded"
		c.Fix = fmt.Sprintf("launchctl bootstrap gui/%d %s", os.Getuid(), plist)
		return c
	}
	c.Status, c.Detail = OK, "loaded"
	if next, ok := nextRun(ctx, env, plist); ok {
		c.Detail += ", next run " + next.Format("Mon 2 Jan 15:04")
	}
	return c
}

type calendarInterval struct {
	Weekday, Hour, Minute *int
}

// nextRun reads StartCalendarInterval from the plist and finds the next
// time it fires. Only Weekday/Hour/Minute entries are understood.
func nextRun(ctx context.Context, env Env, plist string) (time.Time, bool) {
	out, err := env.Run(ctx, "plutil", "-convert", "json", "-o", "-", plist)
	if err != nil {
		return time.Time{}, false
	}
	var p struct {
		StartCalendarInterval []calendarInterval
	}
	if json.Unmarshal(out, &p) != nil {
		return time.Time{}, false
	}
	var best time.Time
	for _, ci := range p.StartCalendarInterval {
		if ci.Weekday == nil || ci.Hour == nil || ci.Minute == nil {
			return time.Time{}, false
		}
		for day := 0; day <= 7; day++ {
			d := env.Now.AddDate(0, 0, day)
			t := time.Date(d.Year(), d.Month(), d.Day(), *ci.Hour, *ci.Minute, 0, 0, env.Now.Location())
			if int(t.Weekday()) == *ci.Weekday%7 && t.After(env.Now) && (best.IsZero() || t.Before(best)) {
				best = t
			}
		}
	}
	return best, !best.IsZero()
}

func lastScheduledRun(env Env) Check {
	c := Check{Name: "Last scheduled run"}
	for _, s := range slices.Backward(env.Scans) {
		if s.Run.Interactive {
			continue
		}
		c.Detail = s.Time.Format("Mon 2 Jan 15:04")
		switch {
		case s.Run.Errors() > 0:
			c.Status = Warn
			c.Detail += ", " + strconv.Itoa(s.Run.Errors()) + " scanner(s) failed"
			c.Fix = "Run gk stats and gk logs --run last --level warn to see why."
		case env.Now.Sub(s.Time) > staleSchedule:
			c.Status = Warn
			c.Detail += ", more than a week ago"
			c.Fix = "Check the Schedule line above, and ~/Library/Logs/groundskeeper.launchd.log."
		default:
			c.Status = OK
		}
		return c
	}
	c.Status, c.Detail = Warn, "none yet"
	c.Fix = "Wait for the next scheduled run, or start one now: launchctl kickstart gui/" + strconv.Itoa(os.Getuid()) + "/" + Label
	return c
}

func fileVault(ctx context.Context, env Env) Check {
	c := Check{Name: "FileVault"}
	out, err := env.Run(ctx, "fdesetup", "status")
	switch {
	case err != nil:
		c.Status, c.Detail = Warn, "couldn't check"
	case strings.Contains(string(out), "FileVault is On"):
		c.Status, c.Detail = OK, "on; reports, snapshots and logs are encrypted at rest"
	default:
		c.Status, c.Detail = Warn, "off; reports, snapshots and logs aren't encrypted at rest"
		c.Fix = "System Settings > Privacy & Security > FileVault > Turn On."
	}
	return c
}

func outputFolder(env Env) Check {
	c := Check{Name: "Report folder"}
	if reason := SyncedFolder(env.OutputDir, env.Home); reason != "" {
		c.Status = Warn
		c.Detail = env.OutputDir + " is in " + reason + ", so reports leave this Mac"
		c.Fix = `Set "outputDir" in ~/.config/groundskeeper/config.json to a folder that doesn't sync, such as ~/Reports/groundskeeper.`
		return c
	}
	c.Status, c.Detail = OK, env.OutputDir+" stays on this Mac"
	return c
}

// logFiles checks that both logs are readable only by the user. launchd
// creates its log with default permissions if it doesn't exist yet.
func logFiles(env Env) Check {
	c := Check{Name: "Log files"}
	var open []string
	for _, name := range []string{"groundskeeper.log", "groundskeeper.launchd.log"} {
		path := filepath.Join(env.Home, "Library", "Logs", name)
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
			open = append(open, path)
		}
	}
	if len(open) == 0 {
		c.Status, c.Detail = OK, "readable only by you"
		return c
	}
	c.Status = Warn
	c.Detail = strings.Join(open, ", ") + " can be read by other users"
	c.Fix = "chmod 600 " + strings.Join(open, " ")
	return c
}

// SyncedFolder returns which syncing folder path is inside, or "". Symlinks
// are resolved first, and a path that doesn't exist yet is judged by its
// nearest existing parent.
func SyncedFolder(path, home string) string {
	synced := []struct{ rel, name string }{
		{"Library/Mobile Documents", "iCloud Drive"},
		{"Library/CloudStorage", "a cloud storage folder"},
		{"Desktop", "Desktop, which iCloud can sync"},
		{"Documents", "Documents, which iCloud can sync"},
	}
	real := resolve(path)
	for _, s := range synced {
		root := resolve(filepath.Join(home, s.rel))
		if real == root || strings.HasPrefix(real, root+string(filepath.Separator)) {
			return s.name
		}
	}
	return ""
}

// resolve follows symlinks in the longest existing prefix of path.
func resolve(path string) string {
	path = filepath.Clean(path)
	rest := ""
	for {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Join(path, rest)
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}
