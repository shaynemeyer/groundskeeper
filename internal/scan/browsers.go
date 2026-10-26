package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// tier is how risky clearing a group of browser folders is.
type tier struct {
	name   string
	safety string
	note   string
}

var (
	tierCache   = tier{"Cache", Safe, "pages and code the browser downloads again when needed"}
	tierOffline = tier{"Offline data", Review, "installed web apps lose their offline copies"}
	tierSite    = tier{"Site data", Keep, "cookies, logins and history; shown for information, clearing it logs you out"}
	tierModel   = tier{"On-device AI model", Review, "Chrome downloads it again unless the on-device AI feature is turned off"}
	tierCrash   = tier{"Crash reports", Review, "crash dumps the browser keeps"}
)

// chromiumBrowser is a browser with Chromium's layout. Its cache folder is
// ~/Library/Caches plus the same relative path as its data folder.
type chromiumBrowser struct {
	name, key, rel string // rel is under ~/Library/Application Support
}

var chromiumBrowsers = []chromiumBrowser{
	{"Chrome", "chrome", "Google/Chrome"},
	{"Edge", "edge", "Microsoft Edge"},
	{"Brave", "brave", "BraveSoftware/Brave-Browser"},
	{"Arc", "arc", "Arc/User Data"},
	{"Chromium", "chromium", "Chromium"},
}

var (
	chromiumCache = []string{"Cache", "Code Cache"}
	chromiumGPU   = "GPUCache"
	chromiumSite  = []string{"Cookies", "Local Storage", "IndexedDB", "History", "Login Data", "Web Data", "Session Storage", "Sessions"}
)

// browserPart is one tier of one browser profile, made of several folders.
type browserPart struct {
	browser, key, profile string
	tier                  tier
	paths                 []string
}

// Browsers measures each installed browser's profiles by tier: cache
// (safe), offline data (review) and site data (keep, never suggested).
type Browsers struct {
	Size     *size.Measurer
	MinBytes int64
}

// Name returns the scanner name.
func (b Browsers) Name() string { return "Browsers" }

// Scan finds every browser profile and measures each tier.
func (b Browsers) Scan(ctx context.Context, home string) (Findings, error) {
	lib := filepath.Join(home, "Library")
	var f Findings
	var parts []browserPart
	for _, br := range chromiumBrowsers {
		parts = append(parts, chromiumParts(lib, br, &f.Skipped)...)
	}
	parts = append(parts, firefoxParts(lib, &f.Skipped)...)
	parts = append(parts,
		browserPart{"Safari", "safari", "", tierCache, []string{
			filepath.Join(lib, "Caches/com.apple.Safari"),
			filepath.Join(lib, "Containers/com.apple.Safari/Data/Library/Caches"),
		}},
		browserPart{"DuckDuckGo", "duckduckgo", "", tierCache, []string{
			filepath.Join(lib, "Caches/com.duckduckgo.macos.browser"),
		}},
	)

	var paths []string
	for _, p := range parts {
		paths = append(paths, p.paths...)
	}
	results := b.Size.MeasureAll(ctx, paths)
	var items []Item
	for _, p := range parts {
		var bytes int64
		for range p.paths {
			bytes += results[0].Bytes
			f.Skipped = append(f.Skipped, results[0].Skipped...)
			results = results[1:]
		}
		items = append(items, p.item(bytes))
	}
	f.Items = keep(items, b.MinBytes)
	return f, nil
}

func (p browserPart) item(bytes int64) Item {
	label := p.browser
	if p.profile != "" {
		label += " · " + p.profile
	}
	path := p.paths[0]
	if len(p.paths) > 1 {
		path = filepath.Dir(path)
	}
	it := Item{
		Section: "Browsers",
		Label:   label + " · " + p.tier.name,
		Path:    path,
		Bytes:   bytes,
		Safety:  p.tier.safety,
		Note:    p.tier.note,
	}
	switch p.tier.safety {
	case Safe:
		it.Hint = "groundskeeper clean browsers --browser " + p.key
	case Review:
		it.Hint = "groundskeeper clean browsers --browser " + p.key + " --include-offline"
	}
	return it
}

func chromiumParts(lib string, br chromiumBrowser, skipped *[]size.Skip) []browserPart {
	support := filepath.Join(lib, "Application Support", br.rel)
	caches := filepath.Join(lib, "Caches", br.rel)
	profiles := profileDirs(support, isChromiumProfile, skipped)
	for _, p := range profileDirs(caches, func(string) bool { return true }, skipped) {
		if !slices.Contains(profiles, p) {
			profiles = append(profiles, p)
		}
	}
	slices.Sort(profiles)

	var parts []browserPart
	for _, p := range profiles {
		cache := join(filepath.Join(caches, p), chromiumCache...)
		cache = append(cache, filepath.Join(support, p, chromiumGPU))
		parts = append(parts,
			browserPart{br.name, br.key, p, tierCache, cache},
			browserPart{br.name, br.key, p, tierOffline, []string{filepath.Join(support, p, "Service Worker")}},
			browserPart{br.name, br.key, p, tierSite, join(filepath.Join(support, p), chromiumSite...)},
		)
	}
	if len(profiles) > 0 {
		parts = append(parts,
			browserPart{br.name, br.key, "", tierModel, []string{filepath.Join(support, "OptGuideOnDeviceModel")}},
			browserPart{br.name, br.key, "", tierCrash, []string{filepath.Join(support, "Crashpad")}},
		)
	}
	return parts
}

func firefoxParts(lib string, skipped *[]size.Skip) []browserPart {
	support := filepath.Join(lib, "Application Support/Firefox/Profiles")
	caches := filepath.Join(lib, "Caches/Firefox/Profiles")
	all := func(string) bool { return true }
	profiles := profileDirs(support, all, skipped)
	for _, p := range profileDirs(caches, all, skipped) {
		if !slices.Contains(profiles, p) {
			profiles = append(profiles, p)
		}
	}
	slices.Sort(profiles)

	var parts []browserPart
	for _, p := range profiles {
		parts = append(parts,
			browserPart{"Firefox", "firefox", p, tierCache, join(filepath.Join(caches, p), "cache2", "startupCache")},
			browserPart{"Firefox", "firefox", p, tierSite, join(filepath.Join(support, p), "storage/default", "cookies.sqlite", "places.sqlite")},
		)
	}
	return parts
}

func isChromiumProfile(name string) bool {
	return name == "Default" || name == "Guest Profile" || name == "System Profile" || strings.HasPrefix(name, "Profile ")
}

// profileDirs lists profile folders in dir. An unreadable dir is recorded
// as skipped, since browsers protect their data without Full Disk Access.
func profileDirs(dir string, match func(string) bool, skipped *[]size.Skip) []string {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		*skipped = append(*skipped, size.NewSkip(dir, err))
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && match(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

// browserCacheRoots are the folders in ~/Library/Caches the Browsers
// scanner owns, so the Caches scanner doesn't count them twice.
func browserCacheRoots(lib string) []string {
	roots := []string{
		filepath.Join(lib, "Caches/Firefox"),
		filepath.Join(lib, "Caches/com.apple.Safari"),
		filepath.Join(lib, "Caches/com.duckduckgo.macos.browser"),
	}
	for _, br := range chromiumBrowsers {
		roots = append(roots, filepath.Join(lib, "Caches", br.rel))
	}
	return roots
}

func join(dir string, names ...string) []string {
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(dir, n)
	}
	return paths
}
