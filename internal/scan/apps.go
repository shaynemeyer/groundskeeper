package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// Apps finds apps not opened in UnusedDays, sized together with the
// leftovers removing them would also free. It only recommends; it never
// removes anything. Apple's apps, running apps and Exclude are never listed.
type Apps struct {
	Size       *size.Measurer
	Run        Runner   // nil runs the real tools
	Dirs       []string // app folders; nil means /Applications and ~/Applications
	UnusedDays int
	Exclude    []string // bundle IDs
	MinBytes   int64
}

type unusedApp struct {
	path, name, id string
	lastUsed       time.Time // zero when Spotlight has no record
	activity       time.Time // newest change in the app's own data
	leftovers      []string
}

// Name returns the scanner name.
func (a Apps) Name() string { return "Unused apps" }

// Scan finds apps whose newest sign of use is older than UnusedDays.
func (a Apps) Scan(ctx context.Context, home string) (Findings, error) {
	run := a.Run
	if run == nil {
		run = RunTool
	}
	dirs := a.Dirs
	if dirs == nil {
		dirs = []string{"/Applications", filepath.Join(home, "Applications")}
	}
	lib := filepath.Join(home, "Library")
	cutoff := time.Now().AddDate(0, 0, -a.UnusedDays)

	paths := findApps(dirs)
	lastUsed := lastUsedDates(ctx, run, paths)
	running := runningPaths(ctx, run)
	var unused []unusedApp
	for i, path := range paths {
		id := bundleID(ctx, run, path)
		if id == "" || strings.HasPrefix(id, "com.apple.") || slices.Contains(a.Exclude, id) || isRunning(path, running) {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".app")
		app := unusedApp{path: path, name: name, id: id, lastUsed: lastUsed[i]}
		app.activity = newestActivity(lib, name, id, cutoff)
		if app.lastUsed.After(cutoff) || app.activity.After(cutoff) {
			continue
		}
		app.leftovers = leftovers(lib, name, id)
		unused = append(unused, app)
	}
	return a.items(ctx, run, unused), nil
}

func (a Apps) items(ctx context.Context, run Runner, unused []unusedApp) Findings {
	var paths []string
	for _, app := range unused {
		paths = append(paths, app.path)
		paths = append(paths, app.leftovers...)
	}
	if len(unused) == 0 {
		return Findings{}
	}
	results := a.Size.MeasureAll(ctx, paths)
	casks := caskApps(ctx, run)

	var f Findings
	var items []Item
	for _, app := range unused {
		appBytes := results[0].Bytes
		var leftBytes int64
		for _, r := range results[:1+len(app.leftovers)] {
			f.Skipped = append(f.Skipped, r.Skipped...)
		}
		for _, r := range results[1 : 1+len(app.leftovers)] {
			leftBytes += r.Bytes
		}
		results = results[1+len(app.leftovers):]

		hint, how := removal(app, casks)
		items = append(items, Item{
			Section: "Unused apps",
			Label:   app.name,
			Path:    app.path,
			Bytes:   appBytes + leftBytes,
			Safety:  Review,
			Hint:    hint,
			Note:    fmt.Sprintf("%s · app %s + leftovers %s · %s", usage(app), ui.Bytes(appBytes), ui.Bytes(leftBytes), how),
		})
	}
	f.Items = keep(items, a.MinBytes)
	return f
}

// usage explains why the app is listed and how sure that is.
func usage(app unusedApp) string {
	days := func(t time.Time) int { return int(time.Since(t).Hours() / 24) }
	switch {
	case !app.lastUsed.IsZero():
		return fmt.Sprintf("last opened %d days ago (high confidence)", days(app.lastUsed))
	case !app.activity.IsZero():
		return fmt.Sprintf("no launch recorded, data last changed %d days ago (low confidence, double-check)", days(app.activity))
	}
	return "no use ever recorded, Spotlight may be off (low confidence, double-check)"
}

// findApps lists .app bundles in each dir and one level of subfolders.
func findApps(dirs []string) []string {
	var apps []string
	for _, dir := range dirs {
		for _, name := range readDirs(dir) {
			path := filepath.Join(dir, name)
			if strings.HasSuffix(name, ".app") {
				apps = append(apps, path)
				continue
			}
			for _, sub := range readDirs(path) {
				if strings.HasSuffix(sub, ".app") && !strings.HasPrefix(sub, "Uninstall") {
					apps = append(apps, filepath.Join(path, sub))
				}
			}
		}
	}
	return apps
}

// leftovers lists the places an app's data lives outside its bundle.
func leftovers(lib, name, id string) []string {
	paths := []string{
		filepath.Join(lib, "Application Support", name),
		filepath.Join(lib, "Caches", id),
		filepath.Join(lib, "Containers", id),
		filepath.Join(lib, "Saved Application State", id+".savedState"),
		filepath.Join(lib, "Logs", name),
		filepath.Join(lib, "Preferences", id+".plist"),
	}
	if id != name {
		paths = append(paths, filepath.Join(lib, "Application Support", id))
	}
	for _, g := range readDirs(filepath.Join(lib, "Group Containers")) {
		if g == "group."+id || strings.HasSuffix(g, "."+id) {
			paths = append(paths, filepath.Join(lib, "Group Containers", g))
		}
	}
	return paths
}

// removal suggests how to remove app based on how it was installed.
func removal(app unusedApp, casks map[string]string) (hint, how string) {
	if token, ok := casks[filepath.Base(app.path)]; ok {
		return "brew uninstall --cask --zap " + ShellQuote(token), "Homebrew cask; --zap also removes leftovers"
	}
	if exists(filepath.Join(app.path, "Contents", "_MASReceipt")) {
		return "", "App Store app; delete it in Launchpad or move it to the Trash, it reinstalls free"
	}
	if u := uninstaller(app.path); u != "" {
		return "open " + ShellQuote(u), "run the vendor's uninstaller"
	}
	return "", "move it to the Trash, then delete the leftovers in ~/Library"
}

func uninstaller(app string) string {
	for _, dir := range []string{filepath.Dir(app), filepath.Join(app, "Contents", "Resources")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "Uninstall") && strings.HasSuffix(e.Name(), ".app") {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}
