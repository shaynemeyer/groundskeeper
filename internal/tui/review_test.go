package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// Two groups: a, b, c with b picked, and d, e with d picked; c is a favorite.
var (
	assets = []iphone.Asset{
		{ID: "a", Created: t0, Bytes: 100},
		{ID: "b", Created: t0.Add(time.Second), Bytes: 200},
		{ID: "c", Created: t0.Add(2 * time.Second), Bytes: 300, Favorite: true},
		{ID: "d", Created: t0.Add(time.Hour), Bytes: 400},
		{ID: "e", Created: t0.Add(time.Hour), Bytes: 500},
	}
	groups = []iphone.Group{
		{IDs: []string{"a", "b", "c"}, Keep: []string{"c"}},
		{IDs: []string{"d", "e"}, Keep: []string{"d"}, Exact: true},
	}
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// review presses keys in a fresh review and reports whether it quit.
func review(t *testing.T, keys ...string) (Review, bool) {
	t.Helper()
	r := NewReview(groups, assets, Viewer{})
	quit := false
	for _, k := range keys {
		m, cmd := r.Update(key(k))
		r = m.(Review)
		quit = cmd != nil
	}
	return r, quit
}

func TestReviewAcceptAll(t *testing.T) {
	r, quit := review(t, "enter", "enter")
	if !quit || !r.Saved() {
		t.Fatalf("quit %v, saved %v", quit, r.Saved())
	}
	if got, want := r.Delete(), []string{"a", "b", "e"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewSkipDeletesNothingFromGroup(t *testing.T) {
	r, _ := review(t, "s", "enter")
	if got, want := r.Delete(), []string{"e"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewChangeKeeper(t *testing.T) {
	// Keep b too in the first group; in the second, keep e and drop d.
	r, _ := review(t, "right", "k", "enter", "right", "k", "left", "k", "enter")
	if got, want := r.Delete(), []string{"a", "d"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewUpDownMoveLikeLeftRight(t *testing.T) {
	// The same edits as TestReviewChangeKeeper, with up and down.
	r, _ := review(t, "down", "k", "enter", "down", "k", "up", "k", "enter")
	if got, want := r.Delete(), []string{"a", "d"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewProtectedStaysKept(t *testing.T) {
	r, _ := review(t, "right", "right", "k")
	if !strings.Contains(r.View().Content, "always kept.") {
		t.Errorf("no note:\n%s", r.View().Content)
	}
	r, _ = review(t, "right", "right", "k", "enter", "enter")
	if slices.Contains(r.Delete(), "c") {
		t.Errorf("favorite deleted: %q", r.Delete())
	}
}

func TestReviewKeepsOnePerGroup(t *testing.T) {
	// d is the only keeper of the second group.
	r, _ := review(t, "s", "k")
	if !strings.Contains(r.View().Content, "at least one photo") {
		t.Errorf("no note:\n%s", r.View().Content)
	}
	r, _ = review(t, "s", "k", "enter")
	if got, want := r.Delete(), []string{"e"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewQuitSavesSoFar(t *testing.T) {
	r, quit := review(t, "enter", "q")
	if !quit || !r.Saved() || !slices.Equal(r.Delete(), []string{"a", "b"}) {
		t.Errorf("quit %v, saved %v, delete %q", quit, r.Saved(), r.Delete())
	}
}

func TestReviewCtrlCDoesNotSave(t *testing.T) {
	r, quit := review(t, "enter", "ctrl+c")
	if !quit || r.Saved() {
		t.Errorf("quit %v, saved %v", quit, r.Saved())
	}
}

func TestReviewBackChangesDecision(t *testing.T) {
	r, _ := review(t, "enter", "p", "s", "enter")
	if got, want := r.Delete(), []string{"e"}; !slices.Equal(got, want) {
		t.Errorf("delete %q, want %q", got, want)
	}
}

func TestReviewView(t *testing.T) {
	r, _ := review(t, "enter")
	view := r.View().Content
	for _, want := range []string{"Group 2 of 2, exact copies", "  > Keep   1 Oct 2026 10:00:00", "Extra  1 Oct 2026 10:00:00", "Accepted so far: 2 photos to delete, 300 B."} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
