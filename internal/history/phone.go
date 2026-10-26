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
	return save(filepath.Join(dir, s.Time.Format(stamp)+".json"), s)
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
