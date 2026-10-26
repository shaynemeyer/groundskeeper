package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

type knownCache struct {
	label string
	rel   string // relative to home
	hint  string // empty: hint is rm -rf of the quoted path
}

var knownCaches = []knownCache{
	{"npm", ".npm", "npm cache clean --force"},
	{"Homebrew", "Library/Caches/Homebrew", "brew cleanup --prune=all"},
	{"pip", "Library/Caches/pip", "pip cache purge"},
	{"Go build cache", "Library/Caches/go-build", "go clean -cache"},
	{"Playwright browsers", "Library/Caches/ms-playwright", "npx playwright uninstall --all"},
	{"Puppeteer browsers", ".cache/puppeteer", ""},
	{"bun", ".bun/install/cache", "bun pm cache rm"},
	{"NuGet packages", ".nuget/packages", "dotnet nuget locals all --clear"},
	{"Cargo registry", ".cargo/registry", ""},
}

// Caches finds package-manager caches and the largest folders in ~/Library/Caches.
type Caches struct {
	Size     *size.Measurer
	MinBytes int64
}

// Name returns the scanner name.
func (c Caches) Name() string { return "Caches" }

// Scan measures known caches and the other children of ~/Library/Caches.
func (c Caches) Scan(ctx context.Context, home string) (Findings, error) {
	var items []Item
	for _, k := range knownCaches {
		path := filepath.Join(home, k.rel)
		hint := k.hint
		if hint == "" {
			hint = "rm -rf " + ShellQuote(path)
		}
		items = append(items, Item{Section: "Caches", Label: k.label, Path: path, Safety: Safe, Hint: hint})
	}
	others, err := otherLibraryCaches(home)
	if err != nil {
		return Findings{}, err
	}
	items = append(items, others...)
	skipped := measure(ctx, c.Size, items)
	return Findings{Items: keep(items, c.MinBytes), Skipped: skipped}, nil
}

func otherLibraryCaches(home string) ([]Item, error) {
	dir := filepath.Join(home, "Library", "Caches")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !e.IsDir() || isKnownCache(home, path) {
			continue
		}
		items = append(items, Item{Section: "Caches", Label: e.Name(), Path: path, Safety: Safe})
	}
	return items, nil
}

func isKnownCache(home, path string) bool {
	return slices.ContainsFunc(knownCaches, func(k knownCache) bool {
		return filepath.Join(home, k.rel) == path
	})
}
