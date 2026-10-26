// Package report renders the HTML report. It uses html/template so file
// names containing markup are escaped.
package report

import (
	_ "embed"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

//go:embed template.html
var templateHTML string

var tmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"bytes": ui.Bytes,
}).Parse(templateHTML))

// Section is one group of items in the report.
type Section struct {
	Name  string
	Items []scan.Item
}

// Bytes is the total size of the section's items.
func (s Section) Bytes() int64 {
	var n int64
	for _, it := range s.Items {
		n += it.Bytes
	}
	return n
}

// Report is everything one run shows.
type Report struct {
	Generated time.Time
	Disk      scan.Disk
	Sections  []Section
	Skipped   []size.Skip
}

// Reclaimable sums the items tagged safe.
func (r Report) Reclaimable() int64 {
	var n int64
	for _, s := range r.Sections {
		for _, it := range s.Items {
			if it.Safety == scan.Safe {
				n += it.Bytes
			}
		}
	}
	return n
}

// UsedPercent is the share of the disk in use.
func (r Report) UsedPercent() int {
	if r.Disk.Total == 0 {
		return 0
	}
	return int(100 * (r.Disk.Total - r.Disk.Free) / r.Disk.Total)
}

// Render writes the report as HTML.
func Render(w io.Writer, r Report) error {
	return tmpl.Execute(w, r)
}

// Write saves the report as YYYY-MM-DD.html and latest.html in dir, readable
// only by the user, and returns the dated path.
func Write(dir string, r Report) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var b strings.Builder
	if err := Render(&b, r); err != nil {
		return "", err
	}
	dated := filepath.Join(dir, r.Generated.Format("2006-01-02")+".html")
	for _, path := range []string{dated, filepath.Join(dir, "latest.html")} {
		if err := writePrivate(path, b.String()); err != nil {
			return "", err
		}
	}
	return dated, nil
}

func writePrivate(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
