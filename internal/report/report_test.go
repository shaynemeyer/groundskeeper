package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
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

func phoneSample() *Phone {
	return &Phone{
		Time: time.Date(2026, 10, 25, 20, 0, 0, 0, time.Local),
		Phone: iphone.Phone{
			IOS:        "27.0.1",
			TotalBytes: 256_000_000_000,
			FreeBytes:  73_000_000_000,
			Apps: []iphone.App{
				{BundleID: "com.example.app1", Name: "Big App", AppBytes: 488_000_000, DataBytes: 8_770_000_000},
				{BundleID: "com.apple.Pages", Name: "Pages", AppBytes: 400_000_000},
			},
			Shared: []iphone.Container{{
				BundleID: "com.example.app2", Name: "Reader", Bytes: 656_900_000, Count: 65,
				Files: []iphone.File{{Name: "<script>alert(1)</script>.mobi", Bytes: 74_900_000}},
			}},
		},
	}
}

func TestRenderShowsIPhone(t *testing.T) {
	r := sample()
	r.IPhone = phoneSample()
	html := render(t, r)
	for _, want := range []string{"iPhone", "iOS 27.0.1", "73.0 GB", "Big App", "com.example.app1", "8.8 GB", "Reader", "65 files", "iPhone Storage"} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("shared file name not escaped")
	}
}

func TestRenderWithoutIPhoneHasNoSection(t *testing.T) {
	if html := render(t, sample()); strings.Contains(html, "iOS ") {
		t.Error("iPhone section shown without iPhone data")
	}
}

func TestRenderShowsUnusedIPhoneApps(t *testing.T) {
	r := sample()
	r.IPhone = phoneSample()
	r.IPhone.UnusedDays = 120
	r.IPhone.UnusedReady = true
	r.IPhone.Scans = 7
	r.IPhone.Unused = map[string]time.Time{"com.example.app1": time.Date(2026, 5, 29, 9, 0, 0, 0, time.Local)}
	html := render(t, r)
	for _, want := range []string{"Probably unused", "29 May 2026", "gk clean iphone-apps &#39;com.example.app1&#39;", "Last Used", "based on 7 iPhone scans"} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestRenderSaysUnusedNeedsHistory(t *testing.T) {
	r := sample()
	r.IPhone = phoneSample()
	r.IPhone.UnusedDays = 120
	html := render(t, r)
	if !strings.Contains(html, "once there are 120 days of iPhone scans") || strings.Contains(html, "Probably unused") {
		t.Error("report should say the unused list needs more history")
	}
}

func photosSample() *iphone.Photos {
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, time.Local) }
	return &iphone.Photos{
		Source: "device",
		Assets: []iphone.Asset{
			{ID: "e1", Created: at(2026, 3, 1, 10, 0), Bytes: 3_000_000},
			{ID: "e2", Created: at(2026, 3, 1, 10, 0), Bytes: 3_000_000},
			{ID: "n1", Created: at(2026, 4, 2, 15, 30), Bytes: 2_000_000, Favorite: true},
			{ID: "n2", Created: at(2026, 4, 2, 15, 30), Bytes: 1_000_000, SidecarBytes: 500_000},
			{ID: "n3", Created: at(2026, 4, 2, 15, 31), Bytes: 2_000_000},
			{ID: "s1", Created: at(2025, 1, 2, 9, 0), Bytes: 400_000, Screenshot: true},
			{ID: "s2", Created: at(2025, 6, 7, 9, 0), Bytes: 600_000, Screenshot: true},
			{ID: "v1", Created: at(2026, 2, 14, 18, 0), Bytes: 50_000_000, SidecarBytes: 5_000_000, Video: true},
		},
		Duplicates: iphone.Duplicates{Groups: []iphone.Group{
			{IDs: []string{"e1", "e2"}, Keep: []string{"e1"}, Exact: true},
			{IDs: []string{"n1", "n2", "n3"}, Keep: []string{"n1"}},
		}},
		OldScreenshots: []string{"s1", "s2"},
		LargestVideos:  []string{"v1"},
	}
}

func TestPhonePhotoTotals(t *testing.T) {
	p := phoneSample()
	p.Photos = photosSample()
	p.ScreenshotDays = 90

	groups := p.PhotoGroups()
	if len(groups) != 2 || groups[0].Freed != 3_000_000 || groups[1].Freed != 3_500_000 {
		t.Fatalf("groups = %+v, want extras freeing 3,000,000 and 3,500,000 with sidecars", groups)
	}
	if !groups[0].Exact || groups[1].Exact || groups[1].Count != 3 {
		t.Errorf("groups = %+v", groups)
	}
	if got := p.DuplicateBytes(); got != 6_500_000 {
		t.Errorf("DuplicateBytes = %d, want 6500000", got)
	}
	s := p.OldScreenshots()
	if s.Count != 2 || s.Bytes != 1_000_000 || s.Days != 90 || s.First.Year() != 2025 || s.Last.Month() != time.June {
		t.Errorf("OldScreenshots = %+v", s)
	}
	if v := p.Videos(); len(v) != 1 || v[0].TotalBytes() != 55_000_000 {
		t.Errorf("Videos = %+v", v)
	}
}

func TestRenderShowsPhotos(t *testing.T) {
	r := sample()
	r.IPhone = phoneSample()
	r.IPhone.Photos = photosSample()
	r.IPhone.ScreenshotDays = 90
	html := render(t, r)
	for _, want := range []string{
		"8 photos and videos, read from the iPhone over USB",
		`<td>1 Mar 2026 10:00</td><td class="num">2</td><td>Exact copies</td><td class="num">3.0 MB</td>`,
		`<td>2 Apr 2026 15:30</td><td class="num">3</td><td>Look alike</td><td class="num">3.5 MB</td>`,
		"2 groups of duplicates; deleting the extras would free 6.5 MB.",
		"2 screenshots older than 90 days take 1.0 MB, taken 2 Jan 2025 to 7 Jun 2025.",
		`<td><span class="path">v1</span></td><td>14 Feb 2026</td><td class="num">55.0 MB</td>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
}
