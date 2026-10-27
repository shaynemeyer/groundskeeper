package clean

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
)

// ErrNoMacReview means no Mac review has been saved.
var ErrNoMacReview = errors.New("no Mac review has been saved")

// MacReview is the Mac items a review ticked. clean mac moves only these,
// and only after checking each against MacRules again.
type MacReview struct {
	Time  time.Time `json:"time"`
	Items []MacItem `json:"items"`
}

// MacItem is one ticked row of the last snapshot. Dir records whether
// Trash was a folder when the review was saved.
type MacItem struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Trash   string `json:"trash"`
	Bytes   int64  `json:"bytes"`
	Dir     bool   `json:"dir"`
}

// MacReviewPath is where the Mac review is saved. Its name doesn't match
// the snapshot pattern, so pruning leaves it alone.
func MacReviewPath(home string) string {
	return filepath.Join(history.Dir(home), "mac-review.json")
}

// NewMacReview builds a review from ticked snapshot rows. Rows without a
// Trash path can't be cleaned, and rows whose path is gone are refused.
func NewMacReview(now time.Time, items []history.Item) (MacReview, error) {
	r := MacReview{Time: now}
	for _, it := range items {
		if it.Trash == "" {
			return MacReview{}, fmt.Errorf("%s can't be moved to the Trash", it.Label)
		}
		info, err := os.Lstat(it.Trash)
		if err != nil {
			return MacReview{}, fmt.Errorf("%s is no longer there", it.Label)
		}
		r.Items = append(r.Items, MacItem{Section: it.Section, Label: it.Label, Trash: it.Trash, Bytes: it.Bytes, Dir: info.IsDir()})
	}
	return r, nil
}

// Bytes is what moving the review's items to the Trash would free.
func (r MacReview) Bytes() int64 {
	var n int64
	for _, it := range r.Items {
		n += it.Bytes
	}
	return n
}

// Without returns r minus the items whose Trash path is in paths, such as
// those just moved.
func (r MacReview) Without(paths []string) MacReview {
	r.Items = slices.DeleteFunc(slices.Clone(r.Items), func(it MacItem) bool { return slices.Contains(paths, it.Trash) })
	return r
}

// SaveMacReview writes r to path, replacing any earlier review.
func SaveMacReview(path string, r MacReview) error {
	return history.SaveJSON(path, r)
}

// LoadMacReview reads the saved review. It returns ErrNoMacReview when
// there is none.
func LoadMacReview(path string) (MacReview, error) {
	var r MacReview
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return r, ErrNoMacReview
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(data, &r)
	return r, err
}

// MacRules decides at clean time whether an item may be moved. The
// allowlist comes from the section rules, never from the review file.
type MacRules struct {
	Home  string
	Roots []string // absolute project roots from the config
}

// Check returns "" if it may be moved to the Trash, or why not.
func (m MacRules) Check(it MacItem) string {
	path := it.Trash
	if filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return "path is not clean"
	}
	if !inside(path, m.Home) {
		return "outside the home folder"
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "no longer there"
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "path is a symlink"
	}
	if real, err := filepath.EvalSymlinks(path); err != nil || real != path {
		return "path goes through a symlink"
	}
	if info.IsDir() != it.Dir {
		return "changed from a folder to a file, or back"
	}
	if !m.allowed(it.Section, path) {
		return "not something " + it.Section + " can clean"
	}
	return ""
}

// allowed applies the section's rule to path.
func (m MacRules) allowed(section, path string) bool {
	parent := filepath.Dir(path)
	switch section {
	case "Caches":
		return scan.CacheTrashable(m.Home, path)
	case "Build artifacts":
		return slices.ContainsFunc(m.Roots, func(root string) bool { return inside(path, root) }) &&
			scan.ArtifactTrashable(path)
	case "Downloads":
		return parent == filepath.Join(m.Home, "Downloads") && !strings.HasPrefix(filepath.Base(path), ".")
	case "AI models":
		lmStudio := filepath.Dir(parent)
		return scan.HFTrashable(filepath.Join(m.Home, ".cache", "huggingface", "hub"), path) ||
			lmStudio == filepath.Join(m.Home, ".lmstudio", "models") ||
			lmStudio == filepath.Join(m.Home, ".cache", "lm-studio", "models")
	}
	return false
}

// MacAction is what happened to one review item.
type MacAction struct {
	Item    MacItem
	Outcome string // Moved, Refused or Failed
	Reason  string // why it was refused or failed
}

// Mac moves the items of a saved review to the Trash, checking each one
// against Rules first.
type Mac struct {
	Rules MacRules
	Run   scan.Runner // runs trash
	Log   *logging.Logger
}

// Clean checks and moves every item of r, one action per item.
func (m Mac) Clean(ctx context.Context, r MacReview) []MacAction {
	var out []MacAction
	for _, it := range r.Items {
		out = append(out, m.act(ctx, it))
	}
	return out
}

func (m Mac) act(ctx context.Context, it MacItem) MacAction {
	a := MacAction{Item: it}
	if reason := m.Rules.Check(it); reason != "" {
		a.Outcome, a.Reason = Refused, reason
		m.Log.Warn("refused", logging.Path(it.Trash), logging.ID("reason", reason))
		return a
	}
	a.Outcome = Moved
	if _, err := m.Run(ctx, trashTool, "-s", it.Trash); err != nil {
		a.Outcome, a.Reason = Failed, "trash could not move it"
	}
	m.Log.Info("item", logging.Path(it.Trash), logging.Bytes(it.Bytes), logging.ID("outcome", a.Outcome))
	return a
}

// inside reports whether path is strictly below dir.
func inside(path, dir string) bool {
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}
