package history

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

func TestSaveAndLoadPhonesPrivately(t *testing.T) {
	home := t.TempDir()
	dir := PhoneDir(home)
	app := iphone.App{BundleID: "com.example.app1", Name: "App 1", AppBytes: 10, DataBytes: 5}
	for _, s := range []PhoneSnapshot{
		{Time: at(26, 9), Phone: iphone.Phone{IOS: "27.0.1", Apps: []iphone.App{app}}},
		{Time: at(22, 9)},
	} {
		if err := SavePhone(dir, s); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Phones(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[1].Time.Equal(at(26, 9)) || got[1].Phone.Apps[0] != app {
		t.Fatalf("got %+v", got)
	}
	for _, p := range []string{dir, filepath.Join(dir, "2026-10-26T0900.json")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if want := os.FileMode(0o600); !info.IsDir() && info.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", p, info.Mode().Perm(), want)
		}
		if want := os.FileMode(0o700); info.IsDir() && info.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", p, info.Mode().Perm(), want)
		}
	}

	scans, err := Scans(Dir(home))
	if err != nil || len(scans) != 0 {
		t.Errorf("Mac scans picked up iPhone snapshots: %+v, %v", scans, err)
	}
}

// phoneWith is a snapshot taken daysAgo before now with one app per data size.
func phoneWith(now time.Time, daysAgo int, data map[string]int64) PhoneSnapshot {
	s := PhoneSnapshot{Time: now.AddDate(0, 0, -daysAgo)}
	for id, n := range data {
		s.Phone.Apps = append(s.Phone.Apps, iphone.App{BundleID: id, DataBytes: n})
	}
	return s
}

// scansEvery returns snapshots every step days from daysAgo back to 1, each
// from data(daysAgo).
func scansEvery(now time.Time, from, step int, data func(daysAgo int) map[string]int64) []PhoneSnapshot {
	var out []PhoneSnapshot
	for d := from; d > 0; d -= step {
		out = append(out, phoneWith(now, d, data(d)))
	}
	return out
}

func TestUnusedFlagsAppsUnchangedForDays(t *testing.T) {
	now := at(26, 9)
	past := scansEvery(now, 150, 25, func(d int) map[string]int64 {
		m := map[string]int64{"com.example.idle": 5, "com.example.busy": 2, "com.apple.Pages": 3, "com.example.kept": 4, "com.example.changed": 1}
		if d >= 100 {
			m["com.example.busy"] = 1 // changed 75 days ago
		} else {
			m["com.example.new"] = 9 // installed 75 days ago
		}
		return m
	})
	current := phoneWith(now, 0, map[string]int64{"com.example.idle": 5, "com.example.busy": 2, "com.example.new": 9, "com.apple.Pages": 3, "com.example.kept": 4, "com.example.changed": 2}).Phone

	unused, ready := Unused(past, current, now, 120, []string{"com.example.kept"})
	if !ready {
		t.Fatal("not ready with 150 days of history")
	}
	want := map[string]time.Time{"com.example.idle": past[0].Time}
	if !maps.Equal(unused, want) {
		t.Errorf("unused = %v, want %v", unused, want)
	}
}

func TestUnusedNeedsScansAtMost30DaysApart(t *testing.T) {
	now := at(26, 9)
	idle := map[string]int64{"com.example.idle": 5}
	current := phoneWith(now, 0, idle).Phone
	for name, days := range map[string][]int{
		"40-day gap in the middle": {150, 125, 100, 60, 35, 10},
		"last scan 40 days ago":    {160, 135, 110, 85, 60, 40},
	} {
		var past []PhoneSnapshot
		for _, d := range days {
			past = append(past, phoneWith(now, d, idle))
		}
		if unused, _ := Unused(past, current, now, 120, nil); len(unused) > 0 {
			t.Errorf("%s: flagged %v", name, unused)
		}
	}
}

func TestUnusedWaitsForHistory(t *testing.T) {
	now := at(26, 9)
	past := []PhoneSnapshot{phoneWith(now, 119, map[string]int64{"com.example.idle": 5})}
	current := phoneWith(now, 0, map[string]int64{"com.example.idle": 5}).Phone
	if unused, ready := Unused(past, current, now, 120, nil); ready || unused != nil {
		t.Errorf("got %v, %v with 119 days of history", unused, ready)
	}
	if _, ready := Unused(nil, current, now, 120, nil); ready {
		t.Error("ready with no history")
	}
}
