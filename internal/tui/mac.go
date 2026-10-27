package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// macChrome is the lines MacList draws besides the rows: the title, the
// totals and the key help.
const macChrome = 7

// MacList lets the user tick snapshot rows to move to the Trash. Only rows
// with a Trash path are listed, grouped by section in report order and
// largest first. Nothing starts ticked.
type MacList struct {
	items  []history.Item
	ticked []bool
	cursor int
	offset int // first line shown
	height int // 0 until the window size is known: show every line
	saved  bool
}

// NewMacList lists the rows of items that can be moved to the Trash.
func NewMacList(items []history.Item) MacList {
	var sections []string
	var rows []history.Item
	for _, it := range items {
		if it.Trash == "" {
			continue
		}
		if !slices.Contains(sections, it.Section) {
			sections = append(sections, it.Section)
		}
		rows = append(rows, it)
	}
	slices.SortStableFunc(rows, func(a, b history.Item) int {
		return cmp.Or(
			cmp.Compare(slices.Index(sections, a.Section), slices.Index(sections, b.Section)),
			cmp.Compare(b.Bytes, a.Bytes))
	})
	return MacList{items: rows, ticked: make([]bool, len(rows))}
}

// Saved reports whether the list ended by saving rather than leaving.
func (m MacList) Saved() bool { return m.saved }

// Len is the number of rows that can be ticked.
func (m MacList) Len() int { return len(m.items) }

// Ticked returns the ticked rows, in list order.
func (m MacList) Ticked() []history.Item {
	var out []history.Item
	for i, it := range m.items {
		if m.ticked[i] {
			out = append(out, it)
		}
	}
	return out
}

func (m MacList) Init() tea.Cmd { return nil }

func (m MacList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = max(msg.Height-macChrome, 1)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, len(m.items)-1)
		case "space":
			if len(m.items) > 0 {
				m.ticked[m.cursor] = !m.ticked[m.cursor]
			}
		case "enter":
			m.saved = true
			return m, tea.Quit
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	m.scroll()
	return m, nil
}

// lines draws every row with its section headings and returns the line
// the cursor is on.
func (m MacList) lines() ([]string, int) {
	var out []string
	at := 0
	for i, it := range m.items {
		if i == 0 || it.Section != m.items[i-1].Section {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, it.Section)
		}
		marker, box := "    ", "[ ]"
		if i == m.cursor {
			marker, at = "  > ", len(out)
		}
		if m.ticked[i] {
			box = "[x]"
		}
		row := fmt.Sprintf("%s%s %9s  %-6s  %s", marker, box, ui.Bytes(it.Bytes), it.Safety, it.Label)
		if it.Note != "" {
			row += "  (" + it.Note + ")"
		}
		out = append(out, row)
	}
	return out, at
}

// scroll keeps the cursor's line in view, and the first heading when the
// cursor is on the first row.
func (m *MacList) scroll() {
	if m.height == 0 {
		return
	}
	_, at := m.lines()
	if at < m.offset {
		m.offset = at
	}
	if at >= m.offset+m.height {
		m.offset = at - m.height + 1
	}
	if m.cursor == 0 {
		m.offset = 0
	}
}

func (m MacList) View() tea.View {
	if len(m.items) == 0 {
		return tea.NewView("Nothing in the last scan can be moved to the Trash.\n")
	}
	lines, _ := m.lines()
	if m.height > 0 {
		lines = lines[m.offset:min(m.offset+m.height, len(lines))]
	}
	var b strings.Builder
	b.WriteString("Tick what to move to the Trash. Nothing is moved until gk clean mac --apply.\n\n")
	b.WriteString(strings.Join(lines, "\n"))
	ticked := m.Ticked()
	var bytes int64
	for _, it := range ticked {
		bytes += it.Bytes
	}
	fmt.Fprintf(&b, "\n\nTicked: %d items, %s.\n", len(ticked), ui.Bytes(bytes))
	b.WriteString("\narrows move, space tick, enter save, q or ctrl+c quit without saving\n")
	return tea.NewView(b.String())
}
