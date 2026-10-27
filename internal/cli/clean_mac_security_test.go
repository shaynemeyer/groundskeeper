package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
)

// applyMacReview saves r, runs clean mac --apply, and returns what trash
// was asked to move and the output.
func applyMacReview(t *testing.T, home string, items ...clean.MacItem) ([]string, string) {
	t.Helper()
	if err := clean.SaveMacReview(clean.MacReviewPath(home), clean.MacReview{Time: scanTime, Items: items}); err != nil {
		t.Fatal(err)
	}
	trashed := fakeTools(t, "")
	out, err := runCleanMacWith(t, "y\n", "--apply")
	if err != nil {
		t.Fatal(err)
	}
	return *trashed, out
}

func mustExist(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestCleanMacRefusesHostileReview(t *testing.T) {
	home := fakeHome(t)
	documents := filepath.Join(home, "Documents")
	taxes := filepath.Join(documents, "taxes")
	if err := os.MkdirAll(taxes, 0o700); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(downloads, "taxes")
	if err := os.Symlink(taxes, link); err != nil {
		t.Fatal(err)
	}
	via := filepath.Join(downloads, "docs")
	if err := os.Symlink(documents, via); err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]clean.MacItem{
		"Documents":       {Section: "Downloads", Label: "documents", Trash: taxes, Dir: true},
		"Documents cache": {Section: "Caches", Label: "documents-cache", Trash: documents, Dir: true},
		"symlink":         {Section: "Downloads", Label: "symlink", Trash: link, Dir: true},
		"through symlink": {Section: "Downloads", Label: "through", Trash: filepath.Join(via, "taxes"), Dir: true},
		"dot dot":         {Section: "Downloads", Label: "dotdot", Trash: downloads + "/../Documents/taxes", Dir: true},
		"outside home":    {Section: "Downloads", Label: "outside", Trash: outside, Dir: true},
	}
	var items []clean.MacItem
	for _, it := range cases {
		items = append(items, it)
	}
	trashed, out := applyMacReview(t, home, items...)

	if len(trashed) != 0 {
		t.Errorf("trash was asked to move %v", trashed)
	}
	mustExist(t, taxes, documents, link, via, outside)
	if !strings.Contains(out, "Moved 0 items") || !strings.Contains(out, "6 items weren't moved") {
		t.Errorf("output:\n%s", out)
	}
	if left := macReviewLeft(t, home); len(left) != len(cases) {
		t.Errorf("review holds %v, want every refused item", left)
	}
}

// reviewNPM scans the fake home and saves a review ticking npm's cache.
func reviewNPM(t *testing.T, home string) string {
	t.Helper()
	run(t, "scan")
	pressMacKeys(t, keySpace, keyEnter)
	run(t, "review", "mac")
	return filepath.Join(home, ".npm")
}

func TestCleanMacRefusesPathSwappedForSymlink(t *testing.T) {
	home := fakeHome(t)
	npm := reviewNPM(t, home)
	documents := filepath.Join(home, "Documents")
	if err := os.MkdirAll(documents, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(npm); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(documents, npm); err != nil {
		t.Fatal(err)
	}

	trashed := fakeTools(t, "")
	out, err := runCleanMacWith(t, "y\n", "--apply")
	if err != nil {
		t.Fatal(err)
	}
	if len(*trashed) != 0 || !strings.Contains(out, "npm: path is a symlink") {
		t.Errorf("trashed %v, output:\n%s", *trashed, out)
	}
	mustExist(t, npm, documents)
}

func TestCleanMacRefusesFolderReplacedByFile(t *testing.T) {
	home := fakeHome(t)
	npm := reviewNPM(t, home)
	if err := os.RemoveAll(npm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npm, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	trashed := fakeTools(t, "")
	out, err := runCleanMacWith(t, "y\n", "--apply")
	if err != nil {
		t.Fatal(err)
	}
	if len(*trashed) != 0 || !strings.Contains(out, "npm: changed from a folder to a file, or back") {
		t.Errorf("trashed %v, output:\n%s", *trashed, out)
	}
	mustExist(t, npm)
}

// The schedule runs exactly scan --notify (TestPlistTemplateIsValidAndRunsReadOnlyScan);
// with a review saved, that run moves nothing and leaves the review alone.
func TestScheduledScanNeverCleansMac(t *testing.T) {
	home := fakeHome(t)
	npm := reviewNPM(t, home)
	before, err := os.ReadFile(clean.MacReviewPath(home))
	if err != nil {
		t.Fatal(err)
	}

	trashed := fakeTools(t, "")
	run(t, "scan", "--notify")
	if len(*trashed) != 0 {
		t.Errorf("scheduled scan ran trash on %v", *trashed)
	}
	mustExist(t, npm)
	after, err := os.ReadFile(clean.MacReviewPath(home))
	if err != nil || string(after) != string(before) {
		t.Errorf("review changed: %v\n%s", err, after)
	}
}
