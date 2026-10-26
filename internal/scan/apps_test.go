package scan

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// fakeMac answers mdls, plutil, ps and brew the way macOS would.
type fakeMac struct {
	ids      map[string]string    // app path -> bundle ID
	lastUsed map[string]time.Time // app path -> last opened
	running  []string
	casks    string
}

func (m fakeMac) run(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "mdls":
		var vals []string
		for _, app := range args[3:] {
			if t, ok := m.lastUsed[app]; ok {
				vals = append(vals, t.UTC().Format(mdlsLayout))
			} else {
				vals = append(vals, "(null)")
			}
		}
		return []byte(strings.Join(vals, "\x00")), nil
	case "plutil":
		app := strings.TrimSuffix(args[len(args)-1], "/Contents/Info.plist")
		if id, ok := m.ids[app]; ok {
			return []byte(id + "\n"), nil
		}
		return nil, errors.New("no plist")
	case "ps":
		return []byte(strings.Join(m.running, "\n")), nil
	case "brew":
		return []byte(m.casks), nil
	}
	return nil, errors.New("unexpected " + name)
}

// ageTree sets every file and folder under root to age, so folder
// modification times don't count as recent activity.
func ageTree(t *testing.T, root string, age time.Duration) {
	t.Helper()
	var paths []string
	filepath.WalkDir(root, func(p string, _ fs.DirEntry, _ error) error {
		paths = append(paths, p)
		return nil
	})
	for i := len(paths) - 1; i >= 0; i-- {
		setAge(t, paths[i], age)
	}
}

func appFixture(t *testing.T) (home, apps string, mac fakeMac) {
	t.Helper()
	home = t.TempDir()
	apps = filepath.Join(home, "Applications")
	lib := filepath.Join(home, "Library")
	mac = fakeMac{ids: map[string]string{}, lastUsed: map[string]time.Time{}}
	add := func(rel, id string) string {
		path := filepath.Join(apps, rel)
		writeFile(t, filepath.Join(path, "Contents/MacOS/bin"), 50_000)
		mac.ids[path] = id
		return path
	}
	old := add("Old.app", "com.example.old")
	mac.lastUsed[old] = time.Now().Add(-200 * day)
	writeFile(t, filepath.Join(lib, "Caches/com.example.old/x"), 100_000)
	writeFile(t, filepath.Join(lib, "Application Support/Old/db"), 10_000)
	ageTree(t, filepath.Join(lib, "Application Support/Old"), 200*day)

	mac.lastUsed[add("Recent.app", "com.example.recent")] = time.Now().Add(-5 * day)

	add("Terminal Tool.app", "com.example.tool")
	writeFile(t, filepath.Join(lib, "Application Support/com.example.tool/state"), 10)

	// Data folder named differently from the app, as with iTerm and iTerm2.
	add("Renamed.app", "com.example.renamed")
	writeFile(t, filepath.Join(lib, "Application Support/Renamed2/state"), 10)
	writeFile(t, filepath.Join(lib, "Preferences/com.example.renamed.plist"), 10)

	add("Ghost.app", "com.example.ghost")
	mac.lastUsed[add("Mail.app", "com.apple.mail")] = time.Now().Add(-500 * day)
	running := add("Busy.app", "com.example.busy")
	mac.running = []string{"/usr/bin/login", running + "/Contents/MacOS/bin"}
	add("Kept.app", "com.example.kept")

	add("Ghostty.app", "com.mitchellh.ghostty")
	mac.casks = `{"casks":[{"token":"ghostty","artifacts":[{"app":["Ghostty.app"]},{"zap":[{"trash":["~/x"]}]}]}]}`
	store := add("Store.app", "com.example.store")
	writeFile(t, filepath.Join(store, "Contents/_MASReceipt/receipt"), 10)
	add("Vendor/Vendor.app", "com.example.vendor")
	mac.ids[filepath.Join(apps, "Vendor/Uninstall Vendor.app")] = "com.example.vendor.uninstaller"
	writeFile(t, filepath.Join(apps, "Vendor/Uninstall Vendor.app/Contents/MacOS/bin"), 10)
	return home, apps, mac
}

func scanApps(t *testing.T, home, apps string, mac fakeMac) map[string]Item {
	t.Helper()
	a := Apps{Size: size.New(time.Minute, 2), Run: mac.run, Dirs: []string{apps}, UnusedDays: 120,
		Exclude: []string{"com.example.kept"}, MinBytes: 1}
	f, err := a.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return byLabel(f.Items)
}

func TestAppsListsOnlyUnusedCandidates(t *testing.T) {
	home, apps, mac := appFixture(t)
	items := scanApps(t, home, apps, mac)

	want := []string{"Old", "Ghost", "Ghostty", "Store", "Vendor"}
	if len(items) != len(want) {
		t.Fatalf("got %v, want %v", keys(items), want)
	}
	for _, w := range want {
		if _, ok := items[w]; !ok {
			t.Errorf("missing %s", w)
		}
	}
}

func TestAppsSizeAndConfidence(t *testing.T) {
	home, apps, mac := appFixture(t)
	items := scanApps(t, home, apps, mac)

	old := items["Old"]
	if old.Bytes < 160_000 || old.Safety != Review {
		t.Errorf("old = %+v, want app plus leftovers", old)
	}
	if !strings.Contains(old.Note, "last opened 200 days ago (high confidence)") || !strings.Contains(old.Note, "leftovers 1") {
		t.Errorf("old note = %q", old.Note)
	}
	if !strings.Contains(items["Ghost"].Note, "low confidence") {
		t.Errorf("ghost note = %q", items["Ghost"].Note)
	}
}

func TestAppsRemovalHintByInstallSource(t *testing.T) {
	home, apps, mac := appFixture(t)
	items := scanApps(t, home, apps, mac)

	if h := items["Ghostty"].Hint; h != "brew uninstall --cask --zap 'ghostty'" {
		t.Errorf("cask hint = %q", h)
	}
	if s := items["Store"]; s.Hint != "" || !strings.Contains(s.Note, "App Store") {
		t.Errorf("store = %+v", s)
	}
	if h := items["Vendor"].Hint; h != "open "+ShellQuote(filepath.Join(apps, "Vendor/Uninstall Vendor.app")) {
		t.Errorf("vendor hint = %q", h)
	}
	if o := items["Old"]; o.Hint != "" || !strings.Contains(o.Note, "move it to the Trash") {
		t.Errorf("old = %+v", o)
	}
}

func TestAppsNoToolsMeansNoGuessing(t *testing.T) {
	home, apps, _ := appFixture(t)
	none := func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("missing") }
	a := Apps{Size: size.New(time.Minute, 2), Run: none, Dirs: []string{apps}, UnusedDays: 120, MinBytes: 1}

	f, err := a.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 0 {
		t.Fatalf("without bundle IDs nothing can be ruled safe to list, got %v", keys(byLabel(f.Items)))
	}
}

func TestLastUsedDatesParsesMdlsOutput(t *testing.T) {
	out := "(null)\x002026-10-25 20:58:06 +0000"
	run := func(context.Context, string, ...string) ([]byte, error) { return []byte(out), nil }
	got := lastUsedDates(context.Background(), run, []string{"/A.app", "/B.app"})
	want := time.Date(2026, 10, 25, 20, 58, 6, 0, time.UTC)
	if !got[0].IsZero() || !got[1].Equal(want) {
		t.Fatalf("got %v", got)
	}
}

func TestAppsHomeWithoutApplications(t *testing.T) {
	a := Apps{Size: size.New(time.Minute, 2), Run: fakeMac{}.run, Dirs: []string{filepath.Join(t.TempDir(), "none")}, UnusedDays: 120}
	f, err := a.Scan(context.Background(), t.TempDir())
	if err != nil || len(f.Items) != 0 {
		t.Fatalf("got %+v, %v", f, err)
	}
}
