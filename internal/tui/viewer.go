package tui

import (
	"io"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

// Viewer shows the review's photos. Either function may be nil.
type Viewer struct {
	// Thumbnails writes JPEGs of ids and returns their paths by ID. IDs it
	// couldn't fetch are left out.
	Thumbnails func(ids []string) (map[string]string, error)
	// Preview builds the command that opens paths in Quick Look.
	Preview func(paths []string) *exec.Cmd
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
