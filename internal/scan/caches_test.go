package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanCaches(t *testing.T, home string, minBytes int64) Findings {
	t.Helper()
	f, err := Caches{Size: size.New(time.Minute, 2), MinBytes: minBytes}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func byLabel(items []Item) map[string]Item {
	m := map[string]Item{}
	for _, it := range items {
		m[it.Label] = it
	}
	return m
}

func TestCachesFindsKnownAndOtherCaches(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".npm", "f"), 300_000)
	writeFile(t, filepath.Join(home, "Library/Caches/Homebrew/f"), 200_000)
	writeFile(t, filepath.Join(home, "Library/Caches/com.example.app/f"), 100_000)

	f := scanCaches(t, home, 1)
	items := byLabel(f.Items)
	if len(f.Items) != 3 {
		t.Fatalf("got %d items, want 3: %+v", len(f.Items), f.Items)
	}
	if items["npm"].Hint != "npm cache clean --force" || items["npm"].Safety != Safe {
		t.Errorf("npm item = %+v", items["npm"])
	}
	if items["Homebrew"].Hint != "brew cleanup --prune=all" {
		t.Errorf("Homebrew item = %+v", items["Homebrew"])
	}
	if items["com.example.app"].Hint != "" {
		t.Errorf("unknown cache should have no hint: %+v", items["com.example.app"])
	}
	if f.Items[0].Label != "npm" {
		t.Errorf("want largest first, got %s", f.Items[0].Label)
	}
}

func TestCachesDropsItemsUnderMinSize(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".npm", "f"), 10_000)

	if f := scanCaches(t, home, 1<<20); len(f.Items) != 0 {
		t.Fatalf("got %+v, want nothing under 1 MB", f.Items)
	}
}

func TestCachesEmptyHome(t *testing.T) {
	if f := scanCaches(t, t.TempDir(), 1); len(f.Items) != 0 || len(f.Skipped) != 0 {
		t.Fatalf("got %+v, want nothing", f)
	}
}

func TestCachesHintQuotesPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "x; rm -rf ~")
	writeFile(t, filepath.Join(home, ".cache/puppeteer/f"), 10_000)

	item := byLabel(scanCaches(t, home, 1).Items)["Puppeteer browsers"]
	want := "rm -rf '" + filepath.Join(home, ".cache/puppeteer") + "'"
	if item.Hint != want {
		t.Fatalf("hint = %q, want %q", item.Hint, want)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":       "'plain'",
		"x; rm -rf ~": "'x; rm -rf ~'",
		"it's":        `'it'\''s'`,
		"$(whoami)":   "'$(whoami)'",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDiskUsage(t *testing.T) {
	d, err := DiskUsage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if d.Total <= 0 || d.Free <= 0 || d.Free > d.Total {
		t.Fatalf("got %+v", d)
	}
}
