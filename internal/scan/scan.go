// Package scan finds reclaimable disk space. Scanners only measure; the
// hints they return are shown to the user and never executed.
package scan

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// Safety tags shown in the report and the terminal.
const (
	Safe   = "safe"
	Review = "review"
	Keep   = "keep"
)

// Item is one reclaimable thing found by a scanner.
type Item struct {
	Section string
	Label   string
	Path    string
	Bytes   int64
	Safety  string
	Hint    string
	Note    string
	Trash   string // set only when moving this one path to the Trash is the whole cleanup
}

// Findings is what a scanner returns.
type Findings struct {
	Items   []Item
	Skipped []size.Skip
}

// Scanner finds items under a home directory.
type Scanner interface {
	Name() string
	Scan(ctx context.Context, home string) (Findings, error)
}

// measure sets each item's Bytes from its Path and returns the skipped paths.
func measure(ctx context.Context, m *size.Measurer, items []Item) []size.Skip {
	paths := make([]string, len(items))
	for i, it := range items {
		paths[i] = it.Path
	}
	var skipped []size.Skip
	for i, res := range m.MeasureAll(ctx, paths) {
		items[i].Bytes = res.Bytes
		skipped = append(skipped, res.Skipped...)
	}
	return skipped
}

// keep drops empty items and items under minBytes, largest first.
func keep(items []Item, minBytes int64) []Item {
	var out []Item
	for _, it := range items {
		if it.Bytes > 0 && it.Bytes >= minBytes {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b Item) int { return cmp.Compare(b.Bytes, a.Bytes) })
	return out
}

// ShellQuote quotes s as a single POSIX shell word, so a crafted file name
// in a copied hint can't run a command.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
