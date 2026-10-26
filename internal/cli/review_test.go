package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/tui"
)

var scanTime = time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local)

// savePhotoScan saves a scan with one group: a and b, keeping b.
func savePhotoScan(t *testing.T, home string, groups ...iphone.Group) {
	t.Helper()
	scan := history.PhotoScan{Time: scanTime, Photos: iphone.Photos{
		Source: iphone.SourceDevice,
		Assets: []iphone.Asset{
			{ID: "F/IMG_0001.HEIC", Created: scanTime.Add(-time.Hour), Bytes: 100, SidecarBytes: 5},
			{ID: "F/IMG_0002.HEIC", Created: scanTime.Add(-time.Hour), Bytes: 200},
		},
		Duplicates: iphone.Duplicates{Groups: groups},
	}}
	if err := history.SavePhotoScan(history.PhotoScanPath(home), scan); err != nil {
		t.Fatal(err)
	}
}

var oneGroup = iphone.Group{IDs: []string{"F/IMG_0001.HEIC", "F/IMG_0002.HEIC"}, Keep: []string{"F/IMG_0002.HEIC"}}

// pressKeys makes the review screen receive keys instead of a terminal.
func pressKeys(t *testing.T, keys ...tea.KeyPressMsg) {
	t.Helper()
	reviewRunner = func(r tui.Review, _ io.Writer) (tui.Review, error) {
		for _, k := range keys {
			m, _ := r.Update(k)
			r = m.(tui.Review)
		}
		return r, nil
	}
	t.Cleanup(func() { reviewRunner = reviewInTerminal })
}

func TestReviewPhotosSavesSelection(t *testing.T) {
	home := fakeHome(t)
	savePhotoScan(t, home, oneGroup)
	pressKeys(t, keyEnter)
	out := run(t, "review", "photos")
	if !strings.Contains(out, "Saved 1 photos to delete, 105 B. Nothing has been deleted.") {
		t.Errorf("output %q", out)
	}
	sel, err := clean.LoadSelection(clean.SelectionPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if sel.Source != iphone.SourceDevice || len(sel.Delete) != 1 || sel.Delete[0].ID != "F/IMG_0001.HEIC" {
		t.Errorf("selection %+v", sel)
	}
}

func TestReviewPhotosCancelSavesNothing(t *testing.T) {
	// Accepting the only group saves at once, so cancel needs two groups.
	home := fakeHome(t)
	savePhotoScan(t, home, oneGroup, oneGroup)
	pressKeys(t, keyEnter, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if out := run(t, "review", "photos"); !strings.Contains(out, "Nothing saved.") {
		t.Errorf("output %q", out)
	}
	if _, err := clean.LoadSelection(clean.SelectionPath(home)); !errors.Is(err, clean.ErrNoSelection) {
		t.Errorf("got %v, want ErrNoSelection", err)
	}
}

func TestReviewPhotosNeedsScan(t *testing.T) {
	fakeHome(t)
	err := execute("review", "photos")
	if !errors.Is(err, history.ErrNoPhotoScan) || !strings.Contains(err.Error(), "run gk iphone first") {
		t.Errorf("got %v", err)
	}
}

func TestReviewPhotosNoDuplicates(t *testing.T) {
	home := fakeHome(t)
	savePhotoScan(t, home)
	if out := run(t, "review", "photos"); !strings.Contains(out, "found no duplicates") {
		t.Errorf("output %q", out)
	}
}

func TestReviewPhotosNeedsTerminal(t *testing.T) {
	// go test has no terminal, so the real runner refuses.
	home := fakeHome(t)
	savePhotoScan(t, home, oneGroup)
	if err := execute("review", "photos"); !errors.Is(err, errNoTerminal) {
		t.Errorf("got %v", err)
	}
}

func TestReviewPhotosListedUnderReview(t *testing.T) {
	cmd, _, err := NewRoot().Find([]string{"review", "photos"})
	if err != nil || cmd.Name() != "photos" || cmd.Parent().GroupID != groupReview {
		t.Errorf("review photos: %v, %v", cmd, err)
	}
}

// execute runs the root command and returns its error.
func execute(args ...string) error {
	root := NewRoot()
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.ExecuteContext(context.Background())
}
