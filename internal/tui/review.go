// Package tui holds the Bubble Tea screens.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// Decisions on a group.
const (
	undecided = iota
	accepted
	skipped
)

// Review steps through duplicate groups. Each starts with the scan's
// keepers; the user accepts a group, changes which photos are kept, or
// skips it. Protected photos are always kept and every group keeps one.
type Review struct {
	groups   []iphone.Group
	assets   map[string]iphone.Asset
	keep     []map[string]bool
	decision []int
	group    int
	cursor   int
	note     string
	saved    bool
	viewer   Viewer
	paths    map[string]string // thumbnail JPEGs by ID
	photoErr string            // why photos can't be shown
}

// NewReview starts a review of groups, whose photos are in assets, shown
// through viewer.
func NewReview(groups []iphone.Group, assets []iphone.Asset, viewer Viewer) Review {
	r := Review{groups: groups, assets: map[string]iphone.Asset{}, decision: make([]int, len(groups)),
		viewer: viewer, paths: map[string]string{}}
	for _, a := range assets {
		r.assets[a.ID] = a
	}
	for _, g := range groups {
		keep := map[string]bool{}
		for _, id := range g.Keep {
			keep[id] = true
		}
		r.keep = append(r.keep, keep)
	}
	return r
}

// Saved reports whether the review ended by saving rather than cancelling.
func (r Review) Saved() bool { return r.saved }

// Delete returns the photos to delete: the ones not kept in accepted groups,
// in group order.
func (r Review) Delete() []string {
	var ids []string
	for i, g := range r.groups {
		if r.decision[i] != accepted {
			continue
		}
		for _, id := range g.IDs {
			if !r.keep[i][id] {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (r Review) Init() tea.Cmd { return r.fetch() }

func (r Review) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case thumbsMsg:
		for id, p := range msg.paths {
			r.paths[id] = p
		}
		if msg.err != nil {
			r.photoErr = "Photos can't be shown: " + msg.err.Error()
		}
		return r, nil
	case previewMsg:
		if msg.err != nil {
			r.note = "Quick Look couldn't open the photos."
		}
		return r, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || len(r.groups) == 0 {
		return r, nil
	}
	r.note = ""
	ids := r.groups[r.group].IDs
	switch key.String() {
	case "up", "left", "h":
		r.cursor = max(r.cursor-1, 0)
	case "down", "right", "l":
		r.cursor = min(r.cursor+1, len(ids)-1)
	case "k":
		r.toggle(ids[r.cursor])
	case "space":
		return r, r.preview()
	case "enter":
		return r.decide(accepted)
	case "s":
		return r.decide(skipped)
	case "p":
		r.move(max(r.group-1, 0))
		return r, r.fetch()
	case "q":
		r.saved = true
		return r, tea.Quit
	case "ctrl+c":
		return r, tea.Quit
	}
	return r, nil
}

// toggle keeps id or makes it an extra, unless that would drop a protected
// photo or the group's last keeper.
func (r *Review) toggle(id string) {
	keep := r.keep[r.group]
	switch {
	case !keep[id]:
		keep[id] = true
	case r.assets[id].Protected():
		r.note = "Favorites, edited photos and photos in albums are always kept."
	case kept(keep) == 1:
		r.note = "Each group keeps at least one photo."
	default:
		keep[id] = false
	}
}

// decide records d for the current group and moves on; after the last
// group the review is saved.
func (r Review) decide(d int) (tea.Model, tea.Cmd) {
	r.decision[r.group] = d
	if r.group == len(r.groups)-1 {
		r.saved = true
		return r, tea.Quit
	}
	r.move(r.group + 1)
	return r, r.fetch()
}

func (r *Review) move(group int) {
	r.group = group
	r.cursor = 0
}

func (r Review) View() tea.View {
	if len(r.groups) == 0 {
		return tea.NewView("No duplicate photos to review.\n")
	}
	g := r.groups[r.group]
	kind := "look alike"
	if g.Exact {
		kind = "exact copies"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Group %d of %d, %s%s\n\n", r.group+1, len(r.groups), kind, decisionNote(r.decision[r.group]))
	for i, id := range g.IDs {
		a := r.assets[id]
		marker, label := "    ", "Extra"
		if i == r.cursor {
			marker = "  > "
		}
		if r.keep[r.group][id] {
			label = "Keep "
		}
		protected := ""
		if a.Protected() {
			protected = "  always kept"
		}
		fmt.Fprintf(&b, "%s%s  %s  %8s%s\n", marker, label, a.Created.Format("2 Jan 2006 15:04:05"), ui.Bytes(a.TotalBytes()), protected)
	}
	count, bytes := r.totals()
	fmt.Fprintf(&b, "\nAccepted so far: %d photos to delete, %s.\n", count, ui.Bytes(bytes))
	if r.photoErr != "" {
		b.WriteString(r.photoErr + "\n")
	}
	if r.note != "" {
		b.WriteString(r.note + "\n")
	}
	b.WriteString("\narrows choose a photo, k keep or not, space preview, enter accept, s skip,\np back, q save and quit, ctrl+c quit without saving\n")
	return tea.NewView(b.String())
}

// totals counts the photos Delete would return and what they'd free.
func (r Review) totals() (int, int64) {
	var n int64
	ids := r.Delete()
	for _, id := range ids {
		n += r.assets[id].TotalBytes()
	}
	return len(ids), n
}

// kept counts the photos marked keep.
func kept(keep map[string]bool) int {
	n := 0
	for _, k := range keep {
		if k {
			n++
		}
	}
	return n
}

func decisionNote(d int) string {
	switch d {
	case accepted:
		return " (accepted)"
	case skipped:
		return " (skipped)"
	}
	return ""
}
