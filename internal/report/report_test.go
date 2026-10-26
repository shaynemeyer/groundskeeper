package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func sample() Report {
	return Report{
		Generated: time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local),
		Disk:      scan.Disk{Free: 100_000_000_000, Total: 500_000_000_000},
		Sections: []Section{{Name: "Caches", Items: []scan.Item{
			{Label: "npm", Path: "/h/.npm", Bytes: 2_000_000_000, Safety: scan.Safe, Hint: "npm cache clean --force"},
			{Label: "other", Path: "/h/x", Bytes: 1_000_000_000, Safety: scan.Review},
		}}},
		Skipped: []size.Skip{{Path: "/h/Library/Mail", Reason: "permission"}},
	}
}

func render(t *testing.T, r Report) string {
	t.Helper()
	var b strings.Builder
	if err := Render(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestReclaimableCountsOnlySafe(t *testing.T) {
	if got := sample().Reclaimable(); got != 2_000_000_000 {
		t.Fatalf("got %d", got)
	}
}

func TestRenderShowsTotalsItemsAndSkips(t *testing.T) {
	html := render(t, sample())
	for _, want := range []string{"100.0 GB", "80%", "2.0 GB", "npm cache clean --force", "/h/Library/Mail", "permission"} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestRenderEscapesFileNames(t *testing.T) {
	r := sample()
	r.Sections[0].Items[0].Label = "<script>alert(1)</script>"
	r.Sections[0].Items[0].Path = `/h/"><img src=x onerror=alert(1)>`

	html := render(t, r)
	if strings.Contains(html, "<script>alert") || strings.Contains(html, "<img src=x") {
		t.Fatal("markup in a file name was not escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("escaped label missing")
	}
}

func TestRenderLoadsNothingExternal(t *testing.T) {
	html := render(t, sample())
	for _, bad := range []string{"http://", "https://", "<script", "<link", "src="} {
		if strings.Contains(html, bad) {
			t.Errorf("report contains %q", bad)
		}
	}
}

func TestWriteDatedAndLatestPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	path, err := Write(dir, sample())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2026-10-26.html" {
		t.Fatalf("path = %s", path)
	}
	for _, p := range []string{path, filepath.Join(dir, "latest.html")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", p, info.Mode().Perm())
		}
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v, want 0700", info.Mode().Perm())
	}
}

func TestWriteTightensExistingFile(t *testing.T) {
	dir := t.TempDir()
	latest := filepath.Join(dir, "latest.html")
	if err := os.WriteFile(latest, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, sample()); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(latest)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", info.Mode().Perm())
	}
}

func withHistory() Report {
	r := sample()
	prev := history.Snapshot{
		Time: time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local),
		Free: 109_000_000_000,
		Items: []history.Item{
			{Section: "", Label: "npm", Path: "/h/.npm", Bytes: 500_000_000},
			{Section: "", Label: "other", Path: "/h/x", Bytes: 1_200_000_000},
		},
	}
	cur := history.Snapshot{Free: r.Disk.Free, Items: []history.Item{
		{Label: "npm", Path: "/h/.npm", Bytes: 2_000_000_000},
		{Label: "other", Path: "/h/x", Bytes: 1_000_000_000},
	}}
	r.Deltas = history.Compare(&prev, cur)
	r.Run = metrics.Run{DurationMS: 25_000, Scanners: []metrics.Scanner{
		{Name: "Caches", DurationMS: 7_000, Items: 2},
		{Name: "Build artifacts", DurationMS: 70_000, Items: 80, Timeouts: 1},
		{Name: "Containers", DurationMS: 200, Error: "timeout"},
	}, Size: size.Stats{Files: 1234, Dirs: 56, HardlinksDeduped: 7}}
	r.Freed = 3_000_000_000
	return r
}

func TestRenderShowsChangesSinceLastRun(t *testing.T) {
	html := render(t, withHistory())
	for _, want := range []string{
		"Free space since Thu 22 Oct 09:00", "-9.0 GB",
		"Growing fastest", `class="num grew">&#43;1.5 GB`, // html/template escapes +
		"-200.0 MB",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestRenderFirstRunHasNoChangeColumn(t *testing.T) {
	html := render(t, sample())
	if strings.Contains(html, "<th>Change</th>") || strings.Contains(html, "Growing fastest") {
		t.Error("first run shows changes")
	}
}

func TestRenderRunDetails(t *testing.T) {
	html := render(t, withHistory())
	for _, want := range []string{
		"Run details", "25.0 s", "1 timed out", "failed (timeout)",
		`<tr class="slow"><td>Build artifacts`, "width: 100%",
		"Visited 1234 files and 56 folders; 7 hardlinks counted once.",
		"Clean commands moved 3.0 GB to the Trash since the last report.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("run details missing %q", want)
		}
	}
	if strings.Contains(html, `<tr class="slow"><td>Caches`) {
		t.Error("fast scanner highlighted")
	}
}
