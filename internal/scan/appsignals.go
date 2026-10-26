package scan

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

const mdlsLayout = "2006-01-02 15:04:05 -0700"

// lastUsedDates asks Spotlight when each app was last opened. mdls -raw
// prints one value per app, NUL-separated, with "(null)" when unknown.
func lastUsedDates(ctx context.Context, run Runner, apps []string) []time.Time {
	dates := make([]time.Time, len(apps))
	if len(apps) == 0 {
		return dates
	}
	out, err := run(ctx, "mdls", append([]string{"-raw", "-name", "kMDItemLastUsedDate"}, apps...)...)
	values := strings.Split(string(out), "\x00")
	if err != nil || len(values) != len(apps) {
		return dates
	}
	for i, v := range values {
		if t, err := time.Parse(mdlsLayout, strings.TrimSpace(v)); err == nil {
			dates[i] = t
		}
	}
	return dates
}

// bundleID reads CFBundleIdentifier from the app's Info.plist, which may
// be binary, so plutil does the parsing.
func bundleID(ctx context.Context, run Runner, app string) string {
	out, err := run(ctx, "plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-",
		filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runningPaths lists the executable path of every running process.
func runningPaths(ctx context.Context, run Runner) []string {
	out, err := run(ctx, "ps", "-axo", "comm=")
	if err != nil {
		return nil
	}
	return strings.Split(string(out), "\n")
}

func isRunning(app string, procs []string) bool {
	for _, p := range procs {
		if strings.HasPrefix(strings.TrimSpace(p), app+"/") {
			return true
		}
	}
	return false
}

// newestActivity returns the newest change in an app's own data, stopping
// early once one is newer than cutoff. Apps opened from a terminal often
// don't update Spotlight's last-used date, so this is the second signal.
// The preferences file is keyed by bundle ID, so it still works when the
// data folder's name differs from the app's (iTerm.app uses iTerm2).
func newestActivity(lib, name, id string, cutoff time.Time) time.Time {
	var newest time.Time
	for _, dir := range []string{
		filepath.Join(lib, "Preferences", id+".plist"),
		filepath.Join(lib, "Application Support", name),
		filepath.Join(lib, "Application Support", id),
		filepath.Join(lib, "Saved Application State", id+".savedState"),
		filepath.Join(lib, "Containers", id),
	} {
		filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			if newest.After(cutoff) {
				return filepath.SkipAll
			}
			return nil
		})
	}
	return newest
}

type caskInfo struct {
	Casks []struct {
		Token     string            `json:"token"`
		Artifacts []json.RawMessage `json:"artifacts"`
	} `json:"casks"`
}

// caskApps maps each installed cask's .app file name to its token.
func caskApps(ctx context.Context, run Runner) map[string]string {
	apps := map[string]string{}
	out, err := run(ctx, "brew", "info", "--cask", "--json=v2", "--installed")
	if err != nil {
		return apps
	}
	var info caskInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return apps
	}
	for _, c := range info.Casks {
		for _, raw := range c.Artifacts {
			var artifact struct {
				App []json.RawMessage `json:"app"`
			}
			if json.Unmarshal(raw, &artifact) != nil {
				continue
			}
			for _, a := range artifact.App {
				var name string
				var target struct {
					Target string `json:"target"`
				}
				if json.Unmarshal(a, &name) == nil {
					apps[filepath.Base(name)] = c.Token
				} else if json.Unmarshal(a, &target) == nil && target.Target != "" {
					apps[filepath.Base(target.Target)] = c.Token
				}
			}
		}
	}
	return apps
}
