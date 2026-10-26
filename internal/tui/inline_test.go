package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

// inlineViewer writes a real JPEG for each photo asked for, or a broken
// file when broken is set, and shows them inline.
func inlineViewer(t *testing.T, broken bool) Viewer {
	t.Helper()
	jpg, err := os.ReadFile(writeJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if broken {
		jpg = []byte("not a jpeg")
	}
	dir := t.TempDir()
	return Viewer{
		Inline: true,
		Thumbnails: func(ids []string) (map[string]string, error) {
			paths := map[string]string{}
			for _, id := range ids {
				paths[id] = filepath.Join(dir, id+".jpg")
				if err := os.WriteFile(paths[id], jpg, 0o600); err != nil {
					return nil, err
				}
			}
			return paths, nil
		},
	}
}

// drain runs cmd and everything it leads to, as the Bubble Tea loop would,
// and returns what was written to the terminal raw.
func drain(r Review, cmd tea.Cmd) (Review, string) {
	if cmd == nil {
		return r, ""
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var raw string
		for _, c := range msg {
			var out string
			r, out = drain(r, c)
			raw += out
		}
		return r, raw
	case tea.RawMsg:
		return r, msg.Msg.(string)
	case nil:
		return r, ""
	default:
		m, next := r.Update(msg)
		return drain(m.(Review), next)
	}
}

// inlineReview starts an inline review in a window width cells wide.
func inlineReview(t *testing.T, broken bool, width int) (Review, string) {
	t.Helper()
	m, _ := NewReview(groups, assets, inlineViewer(t, broken)).Update(tea.WindowSizeMsg{Width: width, Height: 50})
	r := m.(Review)
	return drain(r, r.Init())
}

// shows reports whether view draws image id.
func shows(view string, id int) bool {
	return strings.Contains(view, fmt.Sprintf("\x1b[38;5;%dm%c", id, kitty.Placeholder))
}

func TestReviewInlineShowsGroupImages(t *testing.T) {
	r, raw := inlineReview(t, false, 120)
	for id := 1; id <= 3; id++ {
		if !strings.Contains(raw, fmt.Sprintf("i=%d,", id)) {
			t.Errorf("image %d wasn't sent", id)
		}
		if !shows(r.View().Content, id) {
			t.Errorf("image %d isn't shown", id)
		}
	}
	if view := r.View().Content; !strings.Contains(view, "> Extra") || !strings.Contains(view, "  Keep") {
		t.Errorf("captions missing:\n%s", view)
	}

	m, cmd := r.Update(key("enter"))
	r, raw = drain(m.(Review), cmd)
	view := r.View().Content
	if !strings.Contains(raw, "i=4,") || !shows(view, 4) || !shows(view, 5) || shows(view, 1) {
		t.Errorf("second group should show images 4 and 5 only:\n%q", view)
	}
}

func TestReviewNotInlineSendsNoImages(t *testing.T) {
	v := inlineViewer(t, false)
	v.Inline = false
	m, _ := NewReview(groups, assets, v).Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	r, raw := drain(m.(Review), m.(Review).Init())
	if raw != "" || strings.ContainsRune(r.View().Content, kitty.Placeholder) {
		t.Errorf("images sent or shown without Inline")
	}
}

func TestReviewInlineNarrowWindow(t *testing.T) {
	r, _ := inlineReview(t, false, 60)
	view := r.View().Content
	if strings.ContainsRune(view, kitty.Placeholder) || !strings.Contains(view, "Widen the window") {
		t.Errorf("a narrow window should ask to widen it:\n%s", view)
	}
}

func TestReviewInlineBrokenThumbnail(t *testing.T) {
	r, raw := inlineReview(t, true, 120)
	view := r.View().Content
	if raw != "" || strings.ContainsRune(view, kitty.Placeholder) || !strings.Contains(view, "couldn't be shown here") {
		t.Errorf("a broken thumbnail should be reported, not shown:\n%s", view)
	}
}

func TestReviewInlineQuitDropsAndDeletesImages(t *testing.T) {
	r, _ := inlineReview(t, false, 120)
	m, _ := r.Update(key("q"))
	r = m.(Review)
	if strings.ContainsRune(r.View().Content, kitty.Placeholder) {
		t.Error("the last frame still shows images")
	}
	want := deleteImage(1) + deleteImage(2) + deleteImage(3)
	if got := r.Cleanup(); got != want {
		t.Errorf("Cleanup() = %q, want %q", got, want)
	}
}
