package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func scanBrowsers(t *testing.T, home string) Findings {
	t.Helper()
	f, err := Browsers{Size: size.New(time.Minute, 2), MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func chromeHome(t *testing.T) (home, support string) {
	t.Helper()
	home = t.TempDir()
	lib := filepath.Join(home, "Library")
	support = filepath.Join(lib, "Application Support/Google/Chrome")
	caches := filepath.Join(lib, "Caches/Google/Chrome")
	writeFile(t, filepath.Join(caches, "Default/Cache/data_1"), 30_000)
	writeFile(t, filepath.Join(caches, "Default/Code Cache/js/x"), 20_000)
	writeFile(t, filepath.Join(support, "Default/GPUCache/data_0"), 10_000)
	writeFile(t, filepath.Join(support, "Default/Service Worker/CacheStorage/x"), 40_000)
	writeFile(t, filepath.Join(support, "Default/Cookies"), 10_000)
	writeFile(t, filepath.Join(support, "Default/IndexedDB/x"), 10_000)
	writeFile(t, filepath.Join(caches, "Profile 1/Cache/data_1"), 10_000)
	writeFile(t, filepath.Join(support, "OptGuideOnDeviceModel/weights.bin"), 50_000)
	writeFile(t, filepath.Join(support, "Local State"), 10)
	return home, support
}

func TestBrowsersChromiumTiers(t *testing.T) {
	home, support := chromeHome(t)
	items := byLabel(scanBrowsers(t, home).Items)

	cache := items["Chrome · Default · Cache"]
	if cache.Safety != Safe || cache.Bytes < 60_000 || cache.Hint != "groundskeeper clean browsers --browser chrome" {
		t.Errorf("cache = %+v", cache)
	}
	offline := items["Chrome · Default · Offline data"]
	if offline.Safety != Review || offline.Hint != "groundskeeper clean browsers --browser chrome --include-offline" {
		t.Errorf("offline = %+v", offline)
	}
	site := items["Chrome · Default · Site data"]
	if site.Safety != Keep || site.Hint != "" || site.Path != filepath.Join(support, "Default") {
		t.Errorf("site data = %+v", site)
	}
	if model := items["Chrome · On-device AI model"]; model.Safety != Review || model.Bytes < 50_000 {
		t.Errorf("model = %+v", model)
	}
	if _, ok := items["Chrome · Profile 1 · Cache"]; !ok {
		t.Error("profile found only in the cache folder is missing")
	}
}

func TestBrowsersUnreadableDataFolderIsSkipped(t *testing.T) {
	home, support := chromeHome(t)
	if err := os.Chmod(support, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(support, 0o700) })

	f := scanBrowsers(t, home)
	found := false
	for _, s := range f.Skipped {
		found = found || (s.Path == support && s.Reason == "permission")
	}
	if !found {
		t.Errorf("skipped = %+v, want permission skip for %s", f.Skipped, support)
	}
	if _, ok := byLabel(f.Items)["Chrome · Default · Cache"]; !ok {
		t.Error("cache tier should still be measured from ~/Library/Caches")
	}
}

func TestBrowsersFirefoxSafariDuckDuckGo(t *testing.T) {
	home := t.TempDir()
	lib := filepath.Join(home, "Library")
	writeFile(t, filepath.Join(lib, "Caches/Firefox/Profiles/ab12.default-release/cache2/x"), 10_000)
	writeFile(t, filepath.Join(lib, "Application Support/Firefox/Profiles/ab12.default-release/storage/default/x"), 10_000)
	writeFile(t, filepath.Join(lib, "Caches/com.apple.Safari/x"), 10_000)
	writeFile(t, filepath.Join(lib, "Caches/com.duckduckgo.macos.browser/x"), 10_000)

	items := byLabel(scanBrowsers(t, home).Items)
	for label, safety := range map[string]string{
		"Firefox · ab12.default-release · Cache":     Safe,
		"Firefox · ab12.default-release · Site data": Keep,
		"Safari · Cache":     Safe,
		"DuckDuckGo · Cache": Safe,
	} {
		if it, ok := items[label]; !ok || it.Safety != safety {
			t.Errorf("%s = %+v, want safety %s", label, it, safety)
		}
	}
}

func TestBrowsersNoneInstalled(t *testing.T) {
	if f := scanBrowsers(t, t.TempDir()); len(f.Items) != 0 || len(f.Skipped) != 0 {
		t.Fatalf("got %+v", f)
	}
}

func TestCachesLeavesBrowserFoldersToBrowsers(t *testing.T) {
	home := t.TempDir()
	caches := filepath.Join(home, "Library/Caches")
	writeFile(t, filepath.Join(caches, "Google/Chrome/Default/Cache/x"), 10_000)
	writeFile(t, filepath.Join(caches, "Google/DriveFS/x"), 10_000)
	writeFile(t, filepath.Join(caches, "com.duckduckgo.macos.browser/x"), 10_000)
	writeFile(t, filepath.Join(caches, "com.example.app/x"), 10_000)

	items := byLabel(scanCaches(t, home, 1).Items)
	if len(items) != 2 {
		t.Fatalf("got %v, want Google/DriveFS and com.example.app", keys(items))
	}
	for _, label := range []string{"Google/DriveFS", "com.example.app"} {
		if _, ok := items[label]; !ok {
			t.Errorf("missing %s", label)
		}
	}
}
