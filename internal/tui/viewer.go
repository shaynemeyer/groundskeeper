package tui

import (
	"fmt"
	"io"
	"maps"
	"os/exec"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Viewer shows the review's photos. Either function may be nil.
type Viewer struct {
	// Thumbnails writes JPEGs of ids and returns their paths by ID. IDs it
	// couldn't fetch are left out.
	Thumbnails func(ids []string) (map[string]string, error)
	// Preview builds the command that opens paths in Quick Look.
	Preview func(paths []string) *exec.Cmd
	// Inline shows thumbnails in the screen with Kitty graphics.
	Inline bool
}

// thumbsMsg carries fetched thumbnails, or why they couldn't be fetched.
type thumbsMsg struct {
	paths map[string]string
	err   error
}

// previewMsg says Quick Look has closed.
type previewMsg struct{ err error }

// fetch returns a command that fetches the current group's thumbnails not
// fetched yet, or nil when there's nothing to fetch.
func (r Review) fetch() tea.Cmd {
	if r.viewer.Thumbnails == nil || len(r.groups) == 0 {
		return nil
	}
	var missing []string
	for _, id := range r.groups[r.group].IDs {
		if _, ok := r.paths[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	thumbs := r.viewer.Thumbnails
	return func() tea.Msg {
		paths, err := thumbs(missing)
		return thumbsMsg{paths: paths, err: err}
	}
}

// preview opens the current group's thumbnails in Quick Look, handing it
// the terminal until its window is closed.
func (r *Review) preview() tea.Cmd {
	var paths []string
	for _, id := range r.groups[r.group].IDs {
		if p := r.paths[id]; p != "" {
			paths = append(paths, p)
		}
	}
	if r.viewer.Preview == nil || len(paths) == 0 {
		r.note = "No photos to show for this group yet."
		return nil
	}
	cmd := r.viewer.Preview(paths)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return previewMsg{err: err} })
}

// maxImages is the most images the review sends: IDs go in the 256-color
// foreground, and 0 isn't an ID.
const maxImages = 255

// imageMsg carries a thumbnail's Kitty graphics command, or why it couldn't
// be made.
type imageMsg struct {
	photo string
	image int
	seq   string
	err   error
}

// send encodes new thumbnails for inline display in the background, each
// under the next image ID, until the IDs run out.
func (r *Review) send(paths map[string]string) tea.Cmd {
	if !r.viewer.Inline {
		return nil
	}
	var cmds []tea.Cmd
	for _, id := range slices.Sorted(maps.Keys(paths)) {
		if r.lastImage == maxImages {
			r.photoErr = "Only the first 255 photos are shown here; press space for Quick Look."
			break
		}
		r.lastImage++
		image, path := r.lastImage, paths[id]
		cmds = append(cmds, func() tea.Msg {
			seq, err := sendImage(image, path, thumbCols, thumbRows)
			return imageMsg{photo: id, image: image, seq: seq, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// Cleanup returns the commands that delete the images the review sent,
// for writing to the terminal once the review has ended.
func (r Review) Cleanup() string {
	var b strings.Builder
	for _, image := range slices.Sorted(maps.Values(r.images)) {
		b.WriteString(deleteImage(image))
	}
	return b.String()
}

// imageRow draws the group's thumbnails side by side with Keep or Extra
// under each, or nothing when none has been sent or the review is ending.
func (r Review) imageRow() string {
	ids := r.groups[r.group].IDs
	shown := slices.ContainsFunc(ids, func(id string) bool { return r.images[id] != 0 })
	if !shown || r.quitting {
		return ""
	}
	if len(ids)*(thumbCols+2) > r.width {
		return "Widen the window to see the photos.\n\n"
	}
	lines := make([]string, thumbRows+1)
	for i, id := range ids {
		var cells []string
		if image := r.images[id]; image != 0 {
			cells = placeholders(image, thumbCols, thumbRows)
		}
		for row := range thumbRows {
			cell := strings.Repeat(" ", thumbCols)
			if cells != nil {
				cell = cells[row]
			}
			lines[row] += "  " + cell
		}
		marker, label := "  ", "Extra"
		if i == r.cursor {
			marker = "> "
		}
		if r.keep[r.group][id] {
			label = "Keep"
		}
		lines[thumbRows] += fmt.Sprintf("  %-*s", thumbCols, marker+label)
	}
	return strings.Join(lines, "\n") + "\n\n"
}
