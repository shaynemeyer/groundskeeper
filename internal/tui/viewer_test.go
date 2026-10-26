package tui

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeViewer records the thumbnails asked for and the paths previewed.
type fakeViewer struct {
	asked    [][]string
	previews [][]string
	err      error
}

func (f *fakeViewer) viewer() Viewer {
	return Viewer{
		Thumbnails: func(ids []string) (map[string]string, error) {
			f.asked = append(f.asked, ids)
			paths := map[string]string{}
			for _, id := range ids {
				paths[id] = "/thumbs/" + id + ".jpg"
			}
			if f.err != nil {
				return nil, f.err
			}
			return paths, nil
		},
		Preview: func(paths []string) *exec.Cmd {
			f.previews = append(f.previews, paths)
			return exec.Command("true")
		},
	}
}

// run runs cmd and feeds its message to r, as the Bubble Tea loop would.
func feed(r Review, cmd tea.Cmd) Review {
	if cmd == nil {
		return r
	}
	m, _ := r.Update(cmd())
	return m.(Review)
}

// step presses k and runs any command it returns, except Quick Look's.
func step(r Review, k string) (Review, tea.Cmd) {
	m, cmd := r.Update(key(k))
	r = m.(Review)
	if k == " " {
		return r, cmd
	}
	return feed(r, cmd), nil
}

func TestReviewFetchesEachGroupOnce(t *testing.T) {
	f := &fakeViewer{}
	r := NewReview(groups, assets, f.viewer())
	r = feed(r, r.Init())
	for _, k := range []string{"enter", "p", "enter"} {
		r, _ = step(r, k)
	}
	want := [][]string{{"a", "b", "c"}, {"d", "e"}}
	if !slices.EqualFunc(f.asked, want, slices.Equal) {
		t.Errorf("asked %q, want %q", f.asked, want)
	}
}

func TestReviewPreviewOpensGroupThumbnails(t *testing.T) {
	f := &fakeViewer{}
	r := NewReview(groups, assets, f.viewer())
	r = feed(r, r.Init())
	r, cmd := step(r, " ")
	if cmd == nil {
		t.Fatal("space returned no command")
	}
	want := []string{"/thumbs/a.jpg", "/thumbs/b.jpg", "/thumbs/c.jpg"}
	if len(f.previews) != 1 || !slices.Equal(f.previews[0], want) {
		t.Errorf("previewed %q, want %q", f.previews, want)
	}
}

func TestReviewWithoutThumbnailsStillWorks(t *testing.T) {
	f := &fakeViewer{err: errors.New("the iPhone is locked")}
	r := NewReview(groups, assets, f.viewer())
	r = feed(r, r.Init())
	r, cmd := step(r, " ")
	view := r.View().Content
	if cmd != nil || len(f.previews) != 0 {
		t.Errorf("previewed %q", f.previews)
	}
	for _, want := range []string{"Photos can't be shown: the iPhone is locked", "No photos to show for this group yet."} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	r, _ = step(r, "enter")
	r, _ = step(r, "enter")
	if !r.Saved() || !slices.Equal(r.Delete(), []string{"a", "b", "e"}) {
		t.Errorf("saved %v, delete %q", r.Saved(), r.Delete())
	}
}
