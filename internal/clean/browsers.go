// Package clean holds the opt-in cleanup commands. Each one is a dry run
// unless Apply is set, and logs every action it takes or refuses.
package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

const trashTool = "/usr/bin/trash"

// browserProcesses are the executable names checked before cleaning.
var browserProcesses = map[string]string{
	"chrome": "Google Chrome", "edge": "Microsoft Edge", "brave": "Brave Browser",
	"arc": "Arc", "chromium": "Chromium", "firefox": "firefox",
	"safari": "Safari", "duckduckgo": "DuckDuckGo",
}

// Outcomes recorded for each folder.
const (
	WouldMove = "would move"
	Moved     = "moved to Trash"
	Refused   = "refused"
	Failed    = "failed"
)

// Action is what happened, or would happen, to one folder.
type Action struct {
	Browser, Tier, Path string
	Bytes               int64
	Outcome             string
	Reason              string // why it was refused or failed
}

// Result lists every action and the browsers skipped because they're open.
type Result struct {
	Actions []Action
	Running []string
}

// Browsers moves browser cache folders to the Trash. It only touches the
// cache tier (and the offline tier with Offline), never site data, only
// inside BrowserRoots, and never while that browser is running.
type Browsers struct {
	Home    string
	Keys    []string // browsers to clean; empty means all
	Offline bool
	Apply   bool
	Size    *size.Measurer
	Run     scan.Runner // runs ps and trash
	Log     *logging.Logger
}

// Clean plans or performs the cleanup.
func (b Browsers) Clean(ctx context.Context) (Result, error) {
	for _, k := range b.Keys {
		if !slices.Contains(scan.BrowserKeys(), k) {
			return Result{}, fmt.Errorf("unknown browser %q; choose from %s", k, strings.Join(scan.BrowserKeys(), ", "))
		}
	}
	procs, err := b.Run(ctx, "ps", "-axo", "comm=")
	if err != nil {
		return Result{}, fmt.Errorf("can't check which browsers are running: %w", err)
	}

	var res Result
	roots := realRoots(scan.BrowserRoots(b.Home))
	for _, folders := range b.selected() {
		if isOpen(folders.Key, string(procs)) {
			if !slices.Contains(res.Running, folders.Browser) {
				res.Running = append(res.Running, folders.Browser)
				b.Log.Warn("browser running, skipped", logging.ID("browser", folders.Key))
			}
			continue
		}
		for _, path := range folders.Paths {
			if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			res.Actions = append(res.Actions, b.act(ctx, folders, path, roots))
		}
	}
	return res, nil
}

func (b Browsers) selected() []scan.BrowserFolders {
	var out []scan.BrowserFolders
	for _, f := range scan.BrowserFolderList(b.Home) {
		tierOK := f.Safety == scan.Safe || (b.Offline && f.Safety == scan.Review)
		if tierOK && (len(b.Keys) == 0 || slices.Contains(b.Keys, f.Key)) {
			out = append(out, f)
		}
	}
	return out
}

func (b Browsers) act(ctx context.Context, f scan.BrowserFolders, path string, roots []string) Action {
	a := Action{Browser: f.Browser, Tier: f.Tier, Path: path}
	if reason := allowed(path, roots); reason != "" {
		a.Outcome, a.Reason = Refused, reason
		b.Log.Warn("refused", logging.Path(path), logging.ID("reason", reason))
		return a
	}
	measured := b.Size.Measure(ctx, path)
	a.Bytes = measured.Bytes
	if len(measured.Skipped) > 0 {
		a.Outcome, a.Reason = Refused, "can't read all of it; grant Full Disk Access to clean it"
		b.Log.Warn("refused", logging.Path(path), logging.ID("reason", "unreadable"))
		return a
	}
	a.Outcome = WouldMove
	if b.Apply {
		a.Outcome = Moved
		if _, err := b.Run(ctx, trashTool, "-s", path); err != nil {
			a.Outcome, a.Reason = Failed, "trash could not move it"
		}
	}
	b.Log.Info("folder", logging.Path(path), logging.Bytes(a.Bytes),
		logging.ID("outcome", a.Outcome), logging.Flag("dry_run", !b.Apply))
	return a
}

// allowed returns "" if path may be moved, or why not. The path must be
// clean, must not itself be a symlink, and must resolve inside a root.
func allowed(path string, roots []string) string {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return "path is not clean"
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "can't inspect path"
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "path is a symlink"
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "can't resolve path"
	}
	real := filepath.Join(dir, filepath.Base(path))
	for _, root := range roots {
		if real == root || strings.HasPrefix(real, root+string(filepath.Separator)) {
			return ""
		}
	}
	return "outside the browser folders"
}

func realRoots(roots []string) []string {
	var out []string
	for _, r := range roots {
		if real, err := filepath.EvalSymlinks(r); err == nil {
			out = append(out, real)
		}
	}
	return out
}

func isOpen(key, procs string) bool {
	name := browserProcesses[key]
	for line := range strings.Lines(procs) {
		if filepath.Base(strings.TrimSpace(line)) == name {
			return true
		}
	}
	return false
}

// Total adds up the bytes of actions with the given outcome.
func (r Result) Total(outcome string) int64 {
	var n int64
	for _, a := range r.Actions {
		if a.Outcome == outcome {
			n += a.Bytes
		}
	}
	return n
}
