package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/tui"
)

var keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

// pressMacKeys makes the Mac review screen receive keys instead of a terminal.
func pressMacKeys(t *testing.T, keys ...tea.KeyPressMsg) {
	t.Helper()
	macListRunner = func(l tui.MacList, _ io.Writer) (tui.MacList, error) {
		for _, k := range keys {
			m, _ := l.Update(k)
			l = m.(tui.MacList)
		}
		return l, nil
	}
	t.Cleanup(func() { macListRunner = macListInTerminal })
}

func execReviewMac(t *testing.T) (string, error) {
	t.Helper()
	root := NewRoot()
	root.SetArgs([]string{"review", "mac"})
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

func TestReviewMacSavesTickedItems(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	pressMacKeys(t, keySpace, keyEnter)
	out := run(t, "review", "mac")

	if !strings.Contains(out, "Saved 1 items to move to the Trash") || !strings.Contains(out, "Nothing has been moved") {
		t.Errorf("output %q", out)
	}
	r, err := clean.LoadMacReview(clean.MacReviewPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 1 || r.Items[0].Trash != filepath.Join(home, ".npm") || !r.Items[0].Dir || r.Items[0].Bytes == 0 {
		t.Errorf("review %+v", r)
	}
}

func TestReviewMacQuitSavesNothing(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	pressMacKeys(t, keySpace, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if out := run(t, "review", "mac"); !strings.Contains(out, "Nothing saved.") {
		t.Errorf("output %q", out)
	}
	if _, err := clean.LoadMacReview(clean.MacReviewPath(home)); !errors.Is(err, clean.ErrNoMacReview) {
		t.Errorf("review saved: %v", err)
	}
}

func TestReviewMacNeedsRecentScan(t *testing.T) {
	home := fakeHome(t)
	if _, err := execReviewMac(t); err == nil || !strings.Contains(err.Error(), "run gk scan first") {
		t.Errorf("no scan: err = %v", err)
	}
	old := history.Snapshot{Time: time.Now().Add(-8 * 24 * time.Hour), Items: []history.Item{
		{Section: "Caches", Label: "npm", Bytes: 1, Trash: filepath.Join(home, ".npm")},
	}}
	if err := history.SaveScan(stateDir(home), old); err != nil {
		t.Fatal(err)
	}
	if _, err := execReviewMac(t); err == nil || !strings.Contains(err.Error(), "more than 7 days ago") {
		t.Errorf("old scan: err = %v", err)
	}
}

func TestReviewMacNeedsTerminal(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	_, err := execReviewMac(t)
	if !errors.Is(err, errNoTerminal) || !strings.Contains(err.Error(), "run gk review mac in a terminal") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(clean.MacReviewPath(home)); !os.IsNotExist(err) {
		t.Errorf("review file: %v", err)
	}
}

func TestReviewMacItemGoneSinceScan(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	if err := os.RemoveAll(filepath.Join(home, ".npm")); err != nil {
		t.Fatal(err)
	}
	pressMacKeys(t, keySpace, keyEnter)
	if _, err := execReviewMac(t); err == nil || !strings.Contains(err.Error(), "npm is no longer there; run gk scan and review again") {
		t.Errorf("err = %v", err)
	}
}
