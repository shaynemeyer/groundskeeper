// Package history saves a snapshot of each run in
// ~/.local/state/groundskeeper and compares runs to find what grew.
// The only files it ever deletes are its own expired snapshots.
package history

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

const (
	stamp       = "2006-01-02T1504"
	cleanPrefix = "clean-"
	growingTop  = 5
)

// Item is one report row's size at the time of a snapshot.
type Item struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Path    string `json:"path,omitempty"`
	Bytes   int64  `json:"bytes"`
}

// Key identifies the same row across runs.
func Key(section, label, path string) string {
	return section + "\x00" + label + "\x00" + path
}

// Snapshot is one scan's result.
type Snapshot struct {
	Time  time.Time   `json:"time"`
	Free  int64       `json:"free"`
	Total int64       `json:"total"`
	Items []Item      `json:"items"`
	Run   metrics.Run `json:"run"`
}

// Dir is where snapshots live.
func Dir(home string) string {
	return filepath.Join(home, ".local", "state", "groundskeeper")
}

// SaveScan writes s as YYYY-MM-DDTHHMM.json.
func SaveScan(dir string, s Snapshot) error {
	return save(filepath.Join(dir, s.Time.Format(stamp)+".json"), s)
}

// SaveClean writes c as clean-YYYY-MM-DDTHHMM.json.
func SaveClean(dir string, c metrics.Clean) error {
	return save(filepath.Join(dir, cleanPrefix+c.Time.Format(stamp)+".json"), c)
}

func save(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// Scans returns the scan snapshots in dir, oldest first. Unreadable files
// are left out.
func Scans(dir string) ([]Snapshot, error) {
	var out []Snapshot
	err := eachFile(dir, func(path string, clean bool, _ time.Time) {
		var s Snapshot
		if !clean && load(path, &s) == nil {
			out = append(out, s)
		}
	})
	slices.SortFunc(out, func(a, b Snapshot) int { return a.Time.Compare(b.Time) })
	return out, err
}

// Cleans returns the clean snapshots after since, oldest first.
func Cleans(dir string, since time.Time) ([]metrics.Clean, error) {
	var out []metrics.Clean
	err := eachFile(dir, func(path string, clean bool, _ time.Time) {
		var c metrics.Clean
		if clean && load(path, &c) == nil && c.Time.After(since) {
			out = append(out, c)
		}
	})
	slices.SortFunc(out, func(a, b metrics.Clean) int { return a.Time.Compare(b.Time) })
	return out, err
}

// Prune deletes snapshots named for a time before cutoff. Other files in
// dir are never touched.
func Prune(dir string, cutoff time.Time) (int, error) {
	removed := 0
	err := eachFile(dir, func(path string, _ bool, when time.Time) {
		if when.Before(cutoff) && os.Remove(path) == nil {
			removed++
		}
	})
	return removed, err
}

// eachFile calls fn for regular files named like a snapshot.
func eachFile(dir string, fn func(path string, clean bool, when time.Time)) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		name, clean := strings.CutPrefix(name, cleanPrefix)
		when, err := time.ParseInLocation(stamp, name, time.Local)
		if err != nil {
			continue
		}
		fn(filepath.Join(dir, e.Name()), clean, when)
	}
	return nil
}

func load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Change is how much one row grew or shrank since the previous run.
type Change struct {
	Item
	Delta int64
	New   bool // not in the previous run
}

// Deltas compares a run with the one before it.
type Deltas struct {
	Previous  time.Time // zero when there is no earlier run
	FreeDelta int64
	ByKey     map[string]int64
	Growing   []Change // largest growth first, at most five
}

// HasPrevious reports whether there was an earlier run to compare with.
func (d Deltas) HasPrevious() bool { return !d.Previous.IsZero() }

// Compare works out what changed from prev to cur. A nil prev means this
// is the first run.
func Compare(prev *Snapshot, cur Snapshot) Deltas {
	d := Deltas{ByKey: map[string]int64{}}
	if prev == nil {
		return d
	}
	d.Previous = prev.Time
	d.FreeDelta = cur.Free - prev.Free
	before := map[string]int64{}
	for _, it := range prev.Items {
		before[Key(it.Section, it.Label, it.Path)] = it.Bytes
	}
	for _, it := range cur.Items {
		key := Key(it.Section, it.Label, it.Path)
		old, existed := before[key]
		delta := it.Bytes - old
		d.ByKey[key] = delta
		if delta > 0 {
			d.Growing = append(d.Growing, Change{Item: it, Delta: delta, New: !existed})
		}
	}
	slices.SortFunc(d.Growing, func(a, b Change) int { return cmp.Compare(b.Delta, a.Delta) })
	d.Growing = d.Growing[:min(growingTop, len(d.Growing))]
	return d
}
