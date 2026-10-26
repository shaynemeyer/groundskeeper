package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func scanDownloads(t *testing.T, home string) Findings {
	t.Helper()
	f, err := Downloads{Size: size.New(time.Minute, 2), StaleDays: 90, MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestDownloadsOldEntriesLargestFirst(t *testing.T) {
	home := t.TempDir()
	dl := filepath.Join(home, "Downloads")
	writeFile(t, filepath.Join(dl, "old.dmg"), 50_000)
	setAge(t, filepath.Join(dl, "old.dmg"), 200*day)
	writeFile(t, filepath.Join(dl, "old-folder/a.zip"), 100_000)
	setAge(t, filepath.Join(dl, "old-folder"), 120*day)
	writeFile(t, filepath.Join(dl, "new.pdf"), 50_000)
	writeFile(t, filepath.Join(dl, ".DS_Store"), 10_000)
	setAge(t, filepath.Join(dl, ".DS_Store"), 200*day)

	f := scanDownloads(t, home)
	if len(f.Items) != 2 {
		t.Fatalf("got %v, want the two old entries", keys(byLabel(f.Items)))
	}
	if f.Items[0].Label != "old-folder" || f.Items[0].Note != "downloaded 120 days ago" {
		t.Errorf("first = %+v", f.Items[0])
	}
	if f.Items[1].Label != "old.dmg" || f.Items[1].Safety != Review || f.Items[1].Hint != "" {
		t.Errorf("second = %+v", f.Items[1])
	}
}

func TestDownloadsUsesCreationNotModification(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, "Downloads/fresh.zip")
	writeFile(t, file, 50_000)
	setAge(t, file, 400*day)
	now := time.Now()
	if err := os.Chtimes(file, now, now); err != nil {
		t.Fatal(err)
	}

	items := byLabel(scanDownloads(t, home).Items)
	if it := items["fresh.zip"]; it.Note != "downloaded 400 days ago" {
		t.Fatalf("got %+v; a recent modification must not hide an old download", it)
	}
}

func TestDownloadsUnreadableIsSkipped(t *testing.T) {
	home := t.TempDir()
	dl := filepath.Join(home, "Downloads")
	writeFile(t, filepath.Join(dl, "x"), 10)
	if err := os.Chmod(dl, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dl, 0o700) })

	f := scanDownloads(t, home)
	if len(f.Skipped) != 1 || f.Skipped[0].Path != dl || f.Skipped[0].Reason != "permission" {
		t.Fatalf("skipped = %+v", f.Skipped)
	}
}
