package clean

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

// ErrNoSelection means no review has been saved.
var ErrNoSelection = errors.New("no photo review has been saved")

// Selection is the photos a review approved for deletion. clean photos
// deletes only these, and only from the source they were read from.
type Selection struct {
	Time   time.Time  `json:"time"`
	Source string     `json:"source"`
	Delete []Approved `json:"delete"`
}

// Approved is one photo approved for deletion. ID, Bytes and Created are
// what photos-helper delete checks against the photo before deleting it.
type Approved struct {
	ID           string    `json:"id"`
	Created      time.Time `json:"created"`
	Bytes        int64     `json:"bytes"`
	SidecarBytes int64     `json:"sidecarBytes"`
}

// SelectionPath is where the review selection is saved. Its name doesn't
// match the snapshot pattern, so pruning leaves it alone.
func SelectionPath(home string) string {
	return filepath.Join(history.Dir(home), "photo-review.json")
}

// NewSelection approves ids from a photo scan of source for deletion. It
// refuses IDs the scan doesn't have and photos that are always kept:
// favorites, edited photos and photos in albums.
func NewSelection(now time.Time, source string, assets []iphone.Asset, ids []string) (Selection, error) {
	byID := map[string]iphone.Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	s := Selection{Time: now, Source: source}
	for _, id := range ids {
		a, ok := byID[id]
		if !ok {
			return Selection{}, fmt.Errorf("%q isn't in the photo scan", id)
		}
		if a.Protected() {
			return Selection{}, fmt.Errorf("%q is a favorite, edited or in an album, so it's always kept", id)
		}
		s.Delete = append(s.Delete, Approved{ID: a.ID, Created: a.Created, Bytes: a.Bytes, SidecarBytes: a.SidecarBytes})
	}
	return s, nil
}

// Bytes is what deleting the selection would free.
func (s Selection) Bytes() int64 {
	var n int64
	for _, a := range s.Delete {
		n += a.Bytes + a.SidecarBytes
	}
	return n
}

// SaveSelection writes s to path, replacing any earlier review.
func SaveSelection(path string, s Selection) error {
	return history.SaveJSON(path, s)
}

// LoadSelection reads the saved review. It returns ErrNoSelection when
// there is none.
func LoadSelection(path string) (Selection, error) {
	var s Selection
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, ErrNoSelection
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// Requests is what photos-helper delete needs for each approved photo.
func (s Selection) Requests() []iphone.DeleteRequest {
	var r []iphone.DeleteRequest
	for _, a := range s.Delete {
		r = append(r, iphone.DeleteRequest{ID: a.ID, Bytes: a.Bytes, Created: a.Created})
	}
	return r
}

// Without returns s minus the photos in ids, such as those just deleted.
func (s Selection) Without(ids []string) Selection {
	s.Delete = slices.DeleteFunc(slices.Clone(s.Delete), func(a Approved) bool { return slices.Contains(ids, a.ID) })
	return s
}
