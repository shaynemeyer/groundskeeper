package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/history"
)

var macItems = []history.Item{
	{Section: "Caches", Label: "npm", Bytes: 100, Safety: "safe", Trash: "/h/.npm"},
	{Section: "Caches", Label: "Homebrew", Bytes: 300, Safety: "safe", Trash: "/h/Library/Caches/Homebrew"},
	{Section: "Build artifacts", Label: "p/__pycache__", Bytes: 900, Safety: "safe", Path: "/h/p"},
	{Section: "Build artifacts", Label: "p/node_modules", Bytes: 200, Safety: "review", Trash: "/h/p/node_modules", Note: "commit 40 days ago"},
	{Section: "Toolchains", Label: "rust", Bytes: 5000, Safety: "review"},
}

// macList presses keys in a fresh list and reports whether it quit.
func macList(keys ...string) (MacList, bool) {
	m := NewMacList(macItems)
	quit := false
	for _, k := range keys {
		next, cmd := m.Update(key(k))
		m = next.(MacList)
		quit = cmd != nil
	}
	return m, quit
}

func labels(items []history.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}

func TestMacListOnlyTrashableRowsBySectionLargestFirst(t *testing.T) {
	m := NewMacList(macItems)
	want := "Homebrew npm p/node_modules"
	if got := strings.Join(labels(m.items), " "); got != want {
		t.Errorf("rows %q, want %q", got, want)
	}
	if len(m.Ticked()) != 0 {
		t.Errorf("starts with %v ticked", labels(m.Ticked()))
	}
}

func TestMacListTickAndSave(t *testing.T) {
	m, quit := macList(" ", "down", "down", " ", "up", " ", " ", "enter")
	if !quit || !m.Saved() {
		t.Fatalf("quit %v, saved %v", quit, m.Saved())
	}
	if got := strings.Join(labels(m.Ticked()), " "); got != "Homebrew p/node_modules" {
		t.Errorf("ticked %q", got)
	}
}

func TestMacListQuitWithoutSaving(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m, quit := macList(" ", k)
		if !quit || m.Saved() {
			t.Errorf("%s: quit %v, saved %v", k, quit, m.Saved())
		}
	}
}

func TestMacListView(t *testing.T) {
	m, _ := macList("down", "down", " ")
	view := m.View().Content
	for _, want := range []string{
		"Caches\n  ", "Build artifacts\n",
		"  > [x]     200 B  review  p/node_modules  (commit 40 days ago)",
		"Ticked: 1 items, 200 B.",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "__pycache__") || strings.Contains(view, "rust") {
		t.Errorf("view lists a row without Trash:\n%s", view)
	}
}

func TestMacListScrollsToCursor(t *testing.T) {
	var items []history.Item
	for i := range 30 {
		items = append(items, history.Item{Section: "Downloads", Label: string(rune('a' + i)), Bytes: int64(100 - i), Trash: "/x"})
	}
	m := NewMacList(items)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: macChrome + 5})
	m = next.(MacList)
	for range 20 {
		next, _ = m.Update(key("down"))
		m = next.(MacList)
	}
	view := m.View().Content
	if !strings.Contains(view, "  > [ ]") || strings.Contains(view, "Downloads\n") {
		t.Errorf("cursor row not in view, or view didn't scroll:\n%s", view)
	}
	if got := strings.Count(view, "[ ]"); got != 5 {
		t.Errorf("shows %d rows, want 5:\n%s", got, view)
	}
}
