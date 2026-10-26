package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

type toolchainDir struct {
	rel   string // folder of installed versions, relative to home
	label string // printf-style, given the version folder name
	hint  func(name, path string) string
}

var toolchainDirs = []toolchainDir{
	{".rustup/toolchains", "rustup: %s", func(name, _ string) string {
		return "rustup toolchain uninstall " + ShellQuote(name)
	}},
	// Volta has no command to remove a Node version.
	{".volta/tools/image/node", "Volta: Node %s", func(_, path string) string {
		return "rm -rf " + ShellQuote(path)
	}},
	{".pyenv/versions", "pyenv: Python %s", func(name, _ string) string {
		return "pyenv uninstall " + ShellQuote(name)
	}},
	{".local/share/uv/python", "uv: %s", func(name, _ string) string {
		return "uv python uninstall " + ShellQuote(name)
	}},
}

// Toolchains finds installed rustup toolchains, Volta Node versions, pyenv
// Pythons and uv Pythons. Symlinked versions are aliases and are skipped.
type Toolchains struct {
	Size     *size.Measurer
	MinBytes int64
}

// Name returns the scanner name.
func (t Toolchains) Name() string { return "Toolchains" }

// Scan lists each version folder and measures it.
func (t Toolchains) Scan(ctx context.Context, home string) (Findings, error) {
	var items []Item
	for _, d := range toolchainDirs {
		dir := filepath.Join(home, d.rel)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Findings{}, err
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			items = append(items, Item{
				Section: "Toolchains",
				Label:   strings.Replace(d.label, "%s", e.Name(), 1),
				Path:    path,
				Safety:  Review,
				Hint:    d.hint(e.Name(), path),
			})
		}
	}
	skipped := measure(ctx, t.Size, items)
	return Findings{Items: keep(items, t.MinBytes), Skipped: skipped}, nil
}
