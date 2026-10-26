package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	cellStyle   = lipgloss.NewStyle().Padding(0, 1)
	headerStyle = cellStyle.Bold(true)
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Table renders rows under headers. Columns listed in right are
// right-aligned, as sizes are. Plain output has no border characters.
func Table(headers []string, rows [][]string, right []int, plain bool) string {
	if plain {
		return plainTable(headers, rows, right)
	}
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := cellStyle
			if row == table.HeaderRow {
				s = headerStyle
			}
			if slices.Contains(right, col) {
				s = s.Align(lipgloss.Right)
			}
			return s
		}).
		Render()
}

func plainTable(headers []string, rows [][]string, right []int) string {
	all := append([][]string{headers}, rows...)
	widths := make([]int, len(headers))
	for _, r := range all {
		for i, cell := range r {
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	var b strings.Builder
	for _, r := range all {
		cells := make([]string, len(r))
		for i, cell := range r {
			pad := strings.Repeat(" ", widths[i]-lipgloss.Width(cell))
			if slices.Contains(right, i) {
				cells[i] = pad + cell
			} else {
				cells[i] = cell + pad
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
