package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
)

// saveMacReview saves a review ticking npm's cache and, as a hand-edited
// review might, a folder in ~/Documents filed under Downloads.
func saveMacReview(t *testing.T, home string) (npm, documents string) {
	t.Helper()
	npm = filepath.Join(home, ".npm")
	documents = filepath.Join(home, "Documents/taxes")
	if err := os.MkdirAll(documents, 0o700); err != nil {
		t.Fatal(err)
	}
	r := clean.MacReview{Time: scanTime, Items: []clean.MacItem{
		{Section: "Caches", Label: "npm", Trash: npm, Bytes: 50_000, Dir: true},
		{Section: "Downloads", Label: "taxes", Trash: documents, Bytes: 20_000, Dir: true},
	}}
	if err := clean.SaveMacReview(clean.MacReviewPath(home), r); err != nil {
		t.Fatal(err)
	}
	return npm, documents
}

func runCleanMacWith(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	root := NewRoot()
	root.SetArgs(append([]string{"clean", "mac"}, args...))
	root.SetIn(strings.NewReader(input))
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

func macReviewLeft(t *testing.T, home string) []string {
	t.Helper()
	r, err := clean.LoadMacReview(clean.MacReviewPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, it := range r.Items {
		paths = append(paths, it.Trash)
	}
	return paths
}

func TestCleanMacWithoutReview(t *testing.T) {
	fakeHome(t)
	_, err := runCleanMacWith(t, "")
	if err == nil || !strings.Contains(err.Error(), "run gk review mac first") {
		t.Errorf("err = %v", err)
	}
}

func TestCleanMacDryRunByDefault(t *testing.T) {
	home := fakeHome(t)
	saveMacReview(t, home)
	trashed := fakeTools(t, "")
	out := run(t, "clean", "mac")

	if !strings.Contains(out, "2 items, 70.0 kB, ticked in the review of") || !strings.Contains(out, "Dry run: nothing was changed.") {
		t.Errorf("output = %q", out)
	}
	if len(*trashed) != 0 {
		t.Errorf("dry run trashed %v", *trashed)
	}
}

func TestCleanMacNotConfirmed(t *testing.T) {
	home := fakeHome(t)
	saveMacReview(t, home)
	trashed := fakeTools(t, "")
	out, err := runCleanMacWith(t, "n\n", "--apply")

	if err != nil || !strings.Contains(out, "Move them to the Trash? [y/N]") || !strings.Contains(out, "Nothing was moved.") {
		t.Errorf("err %v, output %q", err, out)
	}
	if len(*trashed) != 0 || len(macReviewLeft(t, home)) != 2 {
		t.Errorf("trashed %v, review %v", *trashed, macReviewLeft(t, home))
	}
}

func TestCleanMacApplyMovesAllowedAndKeepsRefused(t *testing.T) {
	home := fakeHome(t)
	npm, documents := saveMacReview(t, home)
	trashed := fakeTools(t, "")
	out, err := runCleanMacWith(t, "y\n", "--apply")
	if err != nil {
		t.Fatal(err)
	}

	if len(*trashed) != 1 || (*trashed)[0] != npm {
		t.Errorf("trashed %v, want only npm", *trashed)
	}
	if _, err := os.Stat(documents); err != nil {
		t.Errorf("Documents folder: %v", err)
	}
	if left := macReviewLeft(t, home); len(left) != 1 || left[0] != documents {
		t.Errorf("review holds %v, want the refused item", left)
	}
	for _, want := range []string{"Moved 1 items, about 50.0 kB, to the Trash.", "1 items weren't moved", "taxes: not something Downloads can clean"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	cleans, err := history.Cleans(stateDir(home), time.Time{})
	if err != nil || len(cleans) != 1 {
		t.Fatalf("clean snapshots %+v, %v", cleans, err)
	}
	if c := cleans[0]; c.Command != "clean mac" || c.Moved != 1 || c.MovedBytes != 50_000 || c.Refused != 1 || c.DryRun {
		t.Errorf("clean snapshot = %+v", c)
	}
}
