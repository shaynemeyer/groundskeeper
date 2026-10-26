package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

func at(day int, hour int) time.Time {
	return time.Date(2026, 10, day, hour, 0, 0, 0, time.Local)
}

func snap(when time.Time, free int64, items ...Item) Snapshot {
	return Snapshot{Time: when, Free: free, Total: 500, Items: items}
}

func TestSaveAndLoadScansPrivately(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := snap(at(26, 9), 100, Item{Section: "Caches", Label: "npm", Path: "/h/.npm", Bytes: 7})
	s.Run = metrics.Run{Command: "scan", Scanners: []metrics.Scanner{{Name: "Caches", DurationMS: 1500, Items: 1}}}
	if err := SaveScan(dir, snap(at(22, 9), 120)); err != nil {
		t.Fatal(err)
	}
	if err := SaveScan(dir, s); err != nil {
		t.Fatal(err)
	}

	got, err := Scans(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[1].Time.Equal(s.Time) || got[1].Items[0].Bytes != 7 || got[1].Run.Scanners[0].DurationMS != 1500 {
		t.Fatalf("got %+v", got)
	}
	path := filepath.Join(dir, "2026-10-26T0900.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%s: %v %v", path, info, err)
	}
	if d, _ := os.Stat(dir); d.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", d.Mode().Perm())
	}
}

func TestScansSkipsCleanAndBadFiles(t *testing.T) {
	dir := t.TempDir()
	if err := SaveScan(dir, snap(at(26, 9), 100)); err != nil {
		t.Fatal(err)
	}
	if err := SaveClean(dir, metrics.Clean{Time: at(26, 10), Moved: 2, MovedBytes: 50}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "2026-10-27T0900.json"), []byte("{broken"), 0o600)

	scans, _ := Scans(dir)
	cleans, _ := Cleans(dir, at(26, 9))
	if len(scans) != 1 || len(cleans) != 1 || cleans[0].MovedBytes != 50 {
		t.Fatalf("scans %+v cleans %+v", scans, cleans)
	}
	if early, _ := Cleans(dir, at(26, 11)); len(early) != 0 {
		t.Fatalf("cleans before since were returned: %+v", early)
	}
}

func TestPruneOnlyRemovesOwnExpiredSnapshots(t *testing.T) {
	dir := t.TempDir()
	old, recent := time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local), at(20, 9)
	SaveScan(dir, snap(old, 1))
	SaveClean(dir, metrics.Clean{Time: old})
	SaveScan(dir, snap(recent, 1))
	keepers := []string{"notes.txt", "2026-01-01.json", "clean-notes.json", "important.json"}
	for _, name := range keepers {
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)
	}
	os.Mkdir(filepath.Join(dir, "2026-01-01T0900.json"), 0o700)

	removed, err := Prune(dir, time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local))
	if err != nil || removed != 2 {
		t.Fatalf("removed %d, %v; want 2", removed, err)
	}
	for _, name := range append(keepers, recent.Format(stamp)+".json", "2026-01-01T0900.json") {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed", name)
		}
	}
}

func TestCompare(t *testing.T) {
	prev := snap(at(22, 9), 120_000,
		Item{Section: "Caches", Label: "npm", Bytes: 1_000},
		Item{Section: "Caches", Label: "pip", Bytes: 5_000},
		Item{Section: "AI models", Label: "llama", Bytes: 9_000})
	cur := snap(at(26, 9), 111_000,
		Item{Section: "Caches", Label: "npm", Bytes: 4_000},
		Item{Section: "Caches", Label: "pip", Bytes: 2_000},
		Item{Section: "AI models", Label: "qwen", Bytes: 6_000},
		Item{Section: "AI models", Label: "llama", Bytes: 9_000})

	d := Compare(&prev, cur)
	if !d.HasPrevious() || d.FreeDelta != -9_000 {
		t.Fatalf("deltas = %+v", d)
	}
	if d.ByKey[Key("Caches", "npm", "")] != 3_000 || d.ByKey[Key("Caches", "pip", "")] != -3_000 {
		t.Errorf("by key = %v", d.ByKey)
	}
	if len(d.Growing) != 2 || d.Growing[0].Label != "qwen" || !d.Growing[0].New || d.Growing[1].Delta != 3_000 {
		t.Errorf("growing = %+v", d.Growing)
	}
}

func TestCompareCountsNewlySkippedPaths(t *testing.T) {
	prev := snap(at(22, 9), 1)
	prev.Skipped = []string{"/h/Library/Mail", "/h/Downloads"}
	cur := snap(at(26, 9), 1)
	cur.Skipped = []string{"/h/Library/Mail", "/h/Documents", "/h/Desktop"}

	if d := Compare(&prev, cur); d.NewSkip != 2 {
		t.Fatalf("new skipped = %d, want 2", d.NewSkip)
	}
	if d := Compare(nil, cur); d.NewSkip != 0 {
		t.Fatalf("first run counts %d new skipped paths", d.NewSkip)
	}
	old := snap(at(20, 9), 1) // saved before skipped paths were recorded
	if d := Compare(&old, cur); d.NewSkip != 0 {
		t.Fatalf("snapshot without a skipped list counts %d new skipped paths", d.NewSkip)
	}
}

func TestCompareKeepsTopFiveAndHandlesFirstRun(t *testing.T) {
	var prevItems, curItems []Item
	for i := range 8 {
		label := string(rune('a' + i))
		prevItems = append(prevItems, Item{Section: "S", Label: label, Bytes: 0})
		curItems = append(curItems, Item{Section: "S", Label: label, Bytes: int64(i + 1)})
	}
	prev := snap(at(22, 9), 1, prevItems...)
	if d := Compare(&prev, snap(at(26, 9), 1, curItems...)); len(d.Growing) != 5 || d.Growing[0].Delta != 8 {
		t.Errorf("growing = %+v", d.Growing)
	}
	if d := Compare(nil, snap(at(26, 9), 1, curItems...)); d.HasPrevious() || len(d.Growing) != 0 {
		t.Errorf("first run deltas = %+v", d)
	}
}
