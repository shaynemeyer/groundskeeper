package clean

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// fakeMac answers ps and records trash calls without moving anything.
type fakeMac struct {
	procs   string
	psErr   error
	trashed []string
	failOn  string
}

func (m *fakeMac) run(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "ps":
		return []byte(m.procs), m.psErr
	case trashTool:
		path := args[len(args)-1]
		if path == m.failOn {
			return nil, errors.New("exit status 5")
		}
		m.trashed = append(m.trashed, path)
		return nil, nil
	}
	return nil, errors.New("unexpected " + name)
}

func write(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

// browserHome builds Chrome and Firefox profiles with every tier filled.
func browserHome(t *testing.T) (home, caches, support string) {
	t.Helper()
	home = t.TempDir()
	lib := filepath.Join(home, "Library")
	caches = filepath.Join(lib, "Caches/Google/Chrome/Default")
	support = filepath.Join(lib, "Application Support/Google/Chrome/Default")
	write(t, filepath.Join(caches, "Cache/data_1"), 30_000)
	write(t, filepath.Join(caches, "Code Cache/js/x"), 20_000)
	write(t, filepath.Join(support, "GPUCache/data_0"), 10_000)
	write(t, filepath.Join(support, "Service Worker/CacheStorage/x"), 40_000)
	write(t, filepath.Join(support, "Cookies"), 10_000)
	write(t, filepath.Join(support, "Login Data"), 10_000)
	write(t, filepath.Join(lib, "Caches/Firefox/Profiles/p1/cache2/x"), 10_000)
	return home, caches, support
}

func cleaner(home string, mac *fakeMac, apply bool, keys ...string) Browsers {
	return Browsers{Home: home, Keys: keys, Apply: apply, Size: size.New(time.Minute, 2), Run: mac.run, Log: logging.Discard()}
}

func paths(actions []Action) []string {
	var p []string
	for _, a := range actions {
		p = append(p, a.Path)
	}
	return p
}

func TestDryRunListsCacheTierOnlyAndMovesNothing(t *testing.T) {
	home, caches, support := browserHome(t)
	mac := &fakeMac{}
	res, err := cleaner(home, mac, false, "chrome").Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(caches, "Cache"), filepath.Join(caches, "Code Cache"), filepath.Join(support, "GPUCache")}
	if !slices.Equal(paths(res.Actions), want) {
		t.Fatalf("got %v, want %v", paths(res.Actions), want)
	}
	for _, a := range res.Actions {
		if a.Outcome != WouldMove || a.Bytes == 0 {
			t.Errorf("action = %+v", a)
		}
	}
	if len(mac.trashed) != 0 {
		t.Fatalf("dry run trashed %v", mac.trashed)
	}
}

func TestIncludeOfflineAddsServiceWorkerNeverSiteData(t *testing.T) {
	home, _, support := browserHome(t)
	mac := &fakeMac{}
	c := cleaner(home, mac, true)
	c.Offline = true
	res, err := c.Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := paths(res.Actions)
	if !slices.Contains(got, filepath.Join(support, "Service Worker")) {
		t.Errorf("offline tier missing: %v", got)
	}
	for _, p := range append(got, mac.trashed...) {
		if strings.HasSuffix(p, "Cookies") || strings.HasSuffix(p, "Login Data") {
			t.Fatalf("site data touched: %s", p)
		}
	}
}

func TestApplyTrashesEachFolderAndReportsFailures(t *testing.T) {
	home, caches, _ := browserHome(t)
	mac := &fakeMac{failOn: filepath.Join(caches, "Code Cache")}
	res, err := cleaner(home, mac, true, "chrome").Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[string]string{}
	for _, a := range res.Actions {
		outcomes[filepath.Base(a.Path)] = a.Outcome
	}
	if outcomes["Cache"] != Moved || outcomes["GPUCache"] != Moved || outcomes["Code Cache"] != Failed {
		t.Fatalf("outcomes = %v", outcomes)
	}
	if len(mac.trashed) != 2 {
		t.Fatalf("trashed %v", mac.trashed)
	}
}

func TestRefusesRunningBrowser(t *testing.T) {
	home, _, _ := browserHome(t)
	mac := &fakeMac{procs: "/usr/libexec/x\n/Applications/Google Chrome.app/Contents/MacOS/Google Chrome\n"}
	res, err := cleaner(home, mac, true).Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Running, []string{"Chrome"}) {
		t.Fatalf("running = %v", res.Running)
	}
	for _, p := range mac.trashed {
		if strings.Contains(p, "Chrome") {
			t.Fatalf("trashed %s while Chrome was running", p)
		}
	}
	if len(mac.trashed) != 1 || !strings.Contains(mac.trashed[0], "Firefox") {
		t.Fatalf("other browsers should still be cleaned: %v", mac.trashed)
	}
}

func TestCannotCheckProcessesMeansNoCleaning(t *testing.T) {
	home, _, _ := browserHome(t)
	mac := &fakeMac{psErr: errors.New("ps failed")}
	if _, err := cleaner(home, mac, true).Clean(context.Background()); err == nil {
		t.Fatal("want error when running browsers can't be checked")
	}
	if len(mac.trashed) != 0 {
		t.Fatalf("trashed %v", mac.trashed)
	}
}

func TestSymlinkedCacheIsRefusedAndTargetKept(t *testing.T) {
	home, caches, _ := browserHome(t)
	outside := filepath.Join(home, "Documents/important")
	write(t, filepath.Join(outside, "thesis.docx"), 1000)
	cache := filepath.Join(caches, "Cache")
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, cache); err != nil {
		t.Fatal(err)
	}

	mac := &fakeMac{}
	res, err := cleaner(home, mac, true, "chrome").Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range res.Actions {
		if a.Path == cache && (a.Outcome != Refused || a.Reason != "path is a symlink") {
			t.Errorf("symlinked cache = %+v", a)
		}
	}
	if slices.Contains(mac.trashed, cache) {
		t.Fatal("symlinked cache was trashed")
	}
	if _, err := os.Stat(filepath.Join(outside, "thesis.docx")); err != nil {
		t.Fatal("symlink target was touched")
	}
}

func TestSymlinkedProfileIsNeverFollowed(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "Documents")
	write(t, filepath.Join(outside, "Cache/thesis.docx"), 1000)
	profiles := filepath.Join(home, "Library/Caches/Google/Chrome")
	if err := os.MkdirAll(profiles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(profiles, "Default")); err != nil {
		t.Fatal(err)
	}

	mac := &fakeMac{}
	res, err := cleaner(home, mac, true, "chrome").Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Actions) != 0 || len(mac.trashed) != 0 {
		t.Fatalf("followed a symlinked profile: actions %+v, trashed %v", res.Actions, mac.trashed)
	}
}

func TestAllowedRejectsDotDotAndOutsidePaths(t *testing.T) {
	home, caches, _ := browserHome(t)
	roots := realRoots([]string{filepath.Join(home, "Library/Caches/Google/Chrome")})
	cases := map[string]string{
		filepath.Join(caches, "Cache"):                                   "",
		caches + "/../../../../../Documents":                             "path is not clean",
		"Library/Caches/Google/Chrome/Default/Cache":                     "path is not clean",
		filepath.Join(home, "Library/Caches/Firefox/Profiles/p1/cache2"): "outside the browser folders",
	}
	for path, want := range cases {
		if got := allowed(path, roots); got != want {
			t.Errorf("allowed(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestUnreadableCacheIsRefused(t *testing.T) {
	home, caches, _ := browserHome(t)
	locked := filepath.Join(caches, "Cache/index-dir")
	write(t, filepath.Join(locked, "the-real-index"), 10)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	mac := &fakeMac{}
	res, err := cleaner(home, mac, true, "chrome").Clean(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(caches, "Cache")
	for _, a := range res.Actions {
		if a.Path == cache && (a.Outcome != Refused || !strings.Contains(a.Reason, "Full Disk Access")) {
			t.Errorf("unreadable cache = %+v", a)
		}
	}
	if slices.Contains(mac.trashed, cache) {
		t.Fatal("moved a folder it could not fully read")
	}
}

func TestUnknownBrowserIsAnError(t *testing.T) {
	home, _, _ := browserHome(t)
	_, err := cleaner(home, &fakeMac{}, false, "netscape").Clean(context.Background())
	if err == nil || !strings.Contains(err.Error(), "chrome") {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryActionIsLogged(t *testing.T) {
	home, caches, _ := browserHome(t)
	logPath := filepath.Join(t.TempDir(), "gk.log")
	log, closer, err := logging.Open(logPath, logging.Options{Level: "info", Cmd: "clean browsers"})
	if err != nil {
		t.Fatal(err)
	}
	c := cleaner(home, &fakeMac{}, false, "chrome")
	c.Log = log
	if _, err := c.Clean(context.Background()); err != nil {
		t.Fatal(err)
	}
	closer.Close()

	data, _ := os.ReadFile(logPath)
	text := string(data)
	if strings.Count(text, `"outcome":"would move"`) != 3 || !strings.Contains(text, `"dry_run":true`) ||
		!strings.Contains(text, filepath.Join(caches, "Cache")) {
		t.Fatalf("log = %s", text)
	}
}
