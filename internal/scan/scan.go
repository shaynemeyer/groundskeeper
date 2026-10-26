// Package scan finds reclaimable disk space. Scanners only measure; the
// hints they return are shown to the user and never executed.
package scan

import (
	"context"
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

// ShellQuote quotes s as a single POSIX shell word, so a crafted file name
// in a copied hint can't run a command.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
