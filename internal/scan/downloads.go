package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// Downloads finds files and folders in ~/Downloads that arrived more than
// StaleDays ago. Arrival is the creation time, because a download often
// keeps the server's modification time.
type Downloads struct {
	Size      *size.Measurer
	StaleDays int
	MinBytes  int64
}

// Name returns the scanner name.
func (d Downloads) Name() string { return "Downloads" }

// Scan lists the top level of ~/Downloads and measures the old entries.
func (d Downloads) Scan(ctx context.Context, home string) (Findings, error) {
	dir := filepath.Join(home, "Downloads")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Findings{}, nil
	}
	if err != nil {
		return Findings{Skipped: []size.Skip{size.NewSkip(dir, err)}}, nil
	}
	var items []Item
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		days := int(time.Since(created(info)).Hours() / 24)
		if days < d.StaleDays {
			continue
		}
		path := filepath.Join(dir, e.Name())
		items = append(items, Item{
			Section: "Downloads",
			Label:   e.Name(),
			Path:    path,
			Safety:  Review,
			Note:    fmt.Sprintf("downloaded %d days ago", days),
			Trash:   path,
		})
	}
	skipped := measure(ctx, d.Size, items)
	return Findings{Items: keep(items, d.MinBytes), Skipped: skipped}, nil
}

// created returns a file's creation time, or its modification time if the
// file system doesn't record one.
func created(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Birthtimespec.Sec > 0 {
		return time.Unix(st.Birthtimespec.Unix())
	}
	return info.ModTime()
}
