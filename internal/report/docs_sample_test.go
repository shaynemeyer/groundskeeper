package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// docsSample is a report made up for the user guide's screenshots. Every
// name, path and size in it is invented.
func docsSample() Report {
	item := func(section, label, rel string, gb float64, safety, hint, note string) scan.Item {
		return scan.Item{Section: section, Label: label, Path: "/Users/you/" + rel, Bytes: int64(gb * 1e9), Safety: safety, Hint: hint, Note: note}
	}
	sections := []Section{
		{Name: "Caches", Items: []scan.Item{
			item("Caches", "Homebrew", "Library/Caches/Homebrew", 12.6, scan.Safe, "brew cleanup --prune=all", ""),
			item("Caches", "npm", ".npm", 3.4, scan.Safe, "npm cache clean --force", ""),
			item("Caches", "Go build cache", "Library/Caches/go-build", 1.9, scan.Safe, "go clean -cache", ""),
		}},
		{Name: "AI models", Items: []scan.Item{
			item("AI models", "Ollama: llama3:8b", ".ollama/models/manifests/registry.ollama.ai/library/llama3/8b", 4.7, scan.Review, "ollama rm 'llama3:8b'", "pulled 212 days ago"),
			item("AI models", "Hugging Face: model/example-org/tiny-model", ".cache/huggingface/hub/models--example-org--tiny-model", 2.3, scan.Review, "hf cache rm 'model/example-org/tiny-model'", ""),
		}},
		{Name: "Build artifacts", Items: []scan.Item{
			item("Build artifacts", "projects/old-site/node_modules", "projects/old-site/node_modules", 1.2, scan.Safe, "rm -rf '/Users/you/projects/old-site/node_modules'", "last commit 165 days ago"),
			item("Build artifacts", "projects/demo-app/target", "projects/demo-app/target", 2.4, scan.Review, "rm -rf '/Users/you/projects/demo-app/target'", "last commit 3 days ago"),
		}},
		{Name: "Downloads", Items: []scan.Item{
			item("Downloads", "Example-Installer.dmg", "Downloads/Example-Installer.dmg", 1.1, scan.Review, "", "arrived 140 days ago"),
		}},
	}
	cur := history.Snapshot{Free: 123_100_000_000}
	prev := history.Snapshot{Time: time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local), Free: 127_400_000_000}
	for _, s := range sections {
		for _, it := range s.Items {
			cur.Items = append(cur.Items, history.Item{Section: it.Section, Label: it.Label, Path: it.Path, Bytes: it.Bytes})
			before := it.Bytes
			switch it.Label {
			case "Homebrew":
				before -= 2_900_000_000
			case "projects/demo-app/target":
				before -= 1_100_000_000
			}
			prev.Items = append(prev.Items, history.Item{Section: it.Section, Label: it.Label, Path: it.Path, Bytes: before})
		}
	}
	r := Report{
		Generated: time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local),
		Disk:      scan.Disk{Free: cur.Free, Total: 494_400_000_000},
		Sections:  sections,
		Skipped:   []size.Skip{{Path: "/Users/you/Library/Mail", Reason: "permission"}},
		Deltas:    history.Compare(&prev, cur),
		Run: metrics.Run{DurationMS: 31_400, Scanners: []metrics.Scanner{
			{Name: "Caches", DurationMS: 4_100, Items: 3},
			{Name: "AI models", DurationMS: 2_300, Items: 2},
			{Name: "Build artifacts", DurationMS: 21_700, Items: 2},
			{Name: "Downloads", DurationMS: 600, Items: 1},
		}, Size: size.Stats{Files: 812_345, Dirs: 96_210, HardlinksDeduped: 41}},
		IPhone: phoneSample(),
	}
	p := r.IPhone
	p.Phone.Apps = append(p.Phone.Apps,
		iphone.App{BundleID: "com.example.puzzle", Name: "Puzzle Game", AppBytes: 1_900_000_000, DataBytes: 1_200_000_000},
		iphone.App{BundleID: "com.example.trails", Name: "Trail Maps", AppBytes: 310_000_000, DataBytes: 2_400_000_000},
	)
	p.Phone.Shared[0].Files = []iphone.File{{Name: "Example Novel.epub", Bytes: 74_900_000}}
	p.UnusedDays, p.UnusedReady, p.Scans = 120, true, 9
	p.Unused = map[string]time.Time{"com.example.puzzle": time.Date(2026, 5, 29, 9, 0, 0, 0, time.Local)}
	p.Photos = photosSample()
	video := "202602_a/IMG_0412.MOV"
	p.Photos.Assets[len(p.Photos.Assets)-1].ID = video
	p.Photos.LargestVideos = []string{video}
	p.ScreenshotDays = 30
	return r
}

// TestWriteDocsSample writes docsSample to $GK_DOCS_SAMPLE/report.html for
// the user guide's screenshots. It's skipped unless that variable is set.
func TestWriteDocsSample(t *testing.T) {
	dir := os.Getenv("GK_DOCS_SAMPLE")
	if dir == "" {
		t.Skip("set GK_DOCS_SAMPLE to a folder to write the sample report")
	}
	f, err := os.Create(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Render(f, docsSample()); err != nil {
		t.Fatal(err)
	}
}
