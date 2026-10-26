package ui

import (
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"golang.org/x/term"
)

// ANSI colors follow the terminal's own theme in light and dark mode.
var (
	safeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	reviewStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	keepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
	grewStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("2")).Padding(0, 1)
)

// Plain reports whether output to w should have no colors, spinners or boxes:
// when it isn't a terminal, or NO_COLOR is set.
func Plain(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	f, ok := w.(*os.File)
	return !ok || !IsTerminal(f)
}

// IsTerminal reports whether f is a terminal. A character device isn't
// enough: launchd gives jobs /dev/null, which is one but isn't a terminal.
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// Tag renders a safety tag: safe in green, review in yellow, keep in gray.
func Tag(safety string, plain bool) string {
	if plain {
		return safety
	}
	switch safety {
	case "safe":
		return safeStyle.Render(safety)
	case "review":
		return reviewStyle.Render(safety)
	}
	return keepStyle.Render(safety)
}

// Grew renders a size change in red, for something that got bigger.
func Grew(s string, plain bool) string {
	if plain {
		return s
	}
	return grewStyle.Render(s)
}

// Box draws a rounded box around s unless plain.
func Box(s string, plain bool) string {
	if plain {
		return s
	}
	return boxStyle.Render(s)
}
