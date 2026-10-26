package history

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

// PhoneSnapshot is one read of an iPhone.
type PhoneSnapshot struct {
	Time  time.Time    `json:"time"`
	Phone iphone.Phone `json:"phone"`
}

// PhoneDir is where iPhone snapshots live, named like scan snapshots so
// Prune expires them the same way.
func PhoneDir(home string) string {
	return filepath.Join(Dir(home), "iphone")
}

// SavePhone writes s as YYYY-MM-DDTHHMM.json in dir.
func SavePhone(dir string, s PhoneSnapshot) error {
	return SaveJSON(filepath.Join(dir, s.Time.Format(stamp)+".json"), s)
}

// Phones returns the iPhone snapshots in dir, oldest first. Unreadable files
// are left out.
func Phones(dir string) ([]PhoneSnapshot, error) {
	var out []PhoneSnapshot
	err := eachFile(dir, func(path string, clean bool, _ time.Time) {
		var s PhoneSnapshot
		if !clean && load(path, &s) == nil {
			out = append(out, s)
		}
	})
	slices.SortFunc(out, func(a, b PhoneSnapshot) int { return a.Time.Compare(b.Time) })
	return out, err
}

// maxScanGap is the longest gap between iPhone scans an unchanged run may
// span. Sparse scans can't show an app went unused in between.
const maxScanGap = 30 * 24 * time.Hour

// Unused finds apps on phone whose data size has stayed the same, in scans
// no more than maxScanGap apart, for at least days. Each is mapped to the
// first scan with that size. iOS doesn't report Last Used over USB, so this
// is an estimate. ready is false until the oldest snapshot is days old;
// nothing is flagged before. Apple's apps and keep are never flagged.
func Unused(past []PhoneSnapshot, phone iphone.Phone, now time.Time, days int, keep []string) (unused map[string]time.Time, ready bool) {
	cutoff := now.AddDate(0, 0, -days)
	if len(past) == 0 || past[0].Time.After(cutoff) {
		return nil, false
	}
	unused = map[string]time.Time{}
	for _, app := range phone.Apps {
		if app.Apple() || slices.Contains(keep, app.BundleID) {
			continue
		}
		if since := unchangedSince(past, app, now); !since.IsZero() && !since.After(cutoff) {
			unused[app.BundleID] = since
		}
	}
	return unused, true
}

// unchangedSince walks back from now through the snapshots while app's data
// size matches and no gap exceeds maxScanGap. It returns the earliest
// matching snapshot's time, or zero if the newest doesn't qualify.
func unchangedSince(past []PhoneSnapshot, app iphone.App, now time.Time) time.Time {
	var since time.Time
	newer := now
	for i := len(past) - 1; i >= 0; i-- {
		j := slices.IndexFunc(past[i].Phone.Apps, func(a iphone.App) bool { return a.BundleID == app.BundleID })
		if j < 0 || past[i].Phone.Apps[j].DataBytes != app.DataBytes || newer.Sub(past[i].Time) > maxScanGap {
			break
		}
		since, newer = past[i].Time, past[i].Time
	}
	return since
}
