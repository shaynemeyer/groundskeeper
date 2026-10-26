// Package report renders the HTML report. It uses html/template so file
// names containing markup are escaped.
package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

//go:embed template.html
var templateHTML string

var tmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"bytes":   ui.Bytes,
	"delta":   ui.Delta,
	"seconds": func(ms int64) string { return fmt.Sprintf("%.1f s", float64(ms)/1000) },
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
	Deltas    history.Deltas
	Run       metrics.Run
	Freed     int64  // moved to the Trash by clean commands since the last report
	IPhone    *Phone // nil until an iPhone has been read
}

// phoneApps is how many of the iPhone's largest apps the report lists.
const phoneApps = 20

// Phone is the iPhone section: what the latest read of the phone found.
type Phone struct {
	Time        time.Time
	Phone       iphone.Phone
	Unused      map[string]time.Time // bundle ID to when its data size last changed
	UnusedReady bool                 // enough history to flag apps
	UnusedDays  int
	Scans       int // iPhone scans the flag is based on, this one included
	Photos      *iphone.Photos
	PhotosNote  string // why photos weren't scanned, when they weren't
	// ScreenshotDays is the age past which a screenshot counts as old.
	ScreenshotDays int
}

// TopApps is the phone's largest apps.
func (p Phone) TopApps() []iphone.App {
	return p.Phone.Apps[:min(phoneApps, len(p.Phone.Apps))]
}

// PhotoGroup is a duplicate group as the report shows it.
type PhotoGroup struct {
	Taken time.Time // when its oldest photo was taken
	Count int
	Exact bool
	Freed int64 // what deleting the photos not kept would free
}

// Screenshots sums up the old screenshots.
type Screenshots struct {
	Days        int
	Count       int
	Bytes       int64
	First, Last time.Time
}

// PhotoGroups sums up each duplicate group.
func (p Phone) PhotoGroups() []PhotoGroup {
	assets := p.photoAssets()
	var out []PhotoGroup
	for _, g := range p.Photos.Duplicates.Groups {
		group := PhotoGroup{Taken: assets[g.IDs[0]].Created, Count: len(g.IDs), Exact: g.Exact}
		for _, id := range g.IDs {
			if !slices.Contains(g.Keep, id) {
				group.Freed += assets[id].TotalBytes()
			}
		}
		out = append(out, group)
	}
	return out
}

// DuplicateBytes is what deleting every duplicate not kept would free.
func (p Phone) DuplicateBytes() int64 {
	var n int64
	for _, g := range p.PhotoGroups() {
		n += g.Freed
	}
	return n
}

// OldScreenshots sums up the screenshots older than screenshotDays.
func (p Phone) OldScreenshots() Screenshots {
	assets := p.photoAssets()
	s := Screenshots{Days: p.ScreenshotDays}
	for _, id := range p.Photos.OldScreenshots {
		a := assets[id]
		if s.Count == 0 {
			s.First = a.Created
		}
		s.Count++
		s.Bytes += a.TotalBytes()
		s.Last = a.Created
	}
	return s
}

// Videos returns the largest videos, largest first.
func (p Phone) Videos() []iphone.Asset {
	assets := p.photoAssets()
	out := make([]iphone.Asset, len(p.Photos.LargestVideos))
	for i, id := range p.Photos.LargestVideos {
		out[i] = assets[id]
	}
	return out
}

func (p Phone) photoAssets() map[string]iphone.Asset {
	assets := map[string]iphone.Asset{}
	for _, a := range p.Photos.Assets {
		assets[a.ID] = a
	}
	return assets
}

// UnusedApp is an app flagged as probably unused, with a cleanup hint.
type UnusedApp struct {
	iphone.App
	Since time.Time
	Hint  string
}

// UnusedApps lists the probably unused apps, largest first.
func (p Phone) UnusedApps() []UnusedApp {
	var out []UnusedApp
	for _, a := range p.Phone.Apps {
		if since, ok := p.Unused[a.BundleID]; ok {
			out = append(out, UnusedApp{App: a, Since: since, Hint: "gk clean iphone-apps " + scan.ShellQuote(a.BundleID)})
		}
	}
	return out
}

// Change is how much an item changed since the last run, or "" when there
// is nothing to compare with or it didn't change.
func (r Report) Change(it scan.Item) string {
	if d := r.Deltas.ByKey[history.Key(it.Section, it.Label, it.Path)]; d != 0 {
		return ui.Delta(d)
	}
	return ""
}

// Grew reports whether an item is larger than in the last run.
func (r Report) Grew(it scan.Item) bool {
	return r.Deltas.ByKey[history.Key(it.Section, it.Label, it.Path)] > 0
}

// BarWidth is a scanner's share of the slowest scanner's time, in percent.
func (r Report) BarWidth(s metrics.Scanner) int {
	var slowest int64
	for _, sc := range r.Run.Scanners {
		slowest = max(slowest, sc.DurationMS)
	}
	if slowest == 0 {
		return 0
	}
	return int(100 * s.DurationMS / slowest)
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
