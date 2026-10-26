package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

// One-value feature prints: a and b look alike, c doesn't.
var (
	printA = base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0})
	printB = base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0})
	printC = base64.StdEncoding.EncodeToString([]byte{0, 0, 0x80, 0x3f})
)

// fakePhotos makes photoRunner answer each photos-helper command from
// answers, and records the commands run.
func fakePhotos(t *testing.T, answers map[string]string) *[]string {
	t.Helper()
	var ran []string
	photoRunner = func(_ context.Context, _ []byte, _ string, args ...string) ([]byte, error) {
		ran = append(ran, args[0])
		answer, ok := answers[args[0]]
		if !ok {
			return nil, errors.New("unexpected command " + args[0])
		}
		if strings.Contains(answer, `"error"`) {
			return []byte(answer), errors.New("exit status 1")
		}
		return []byte(answer), nil
	}
	t.Cleanup(func() { photoRunner = iphone.RunHelper })
	return &ran
}

var phonePhotos = map[string]string{
	"list": `{"source":"device","assets":[
		{"id":"F/IMG_0001.HEIC","created":"2026-10-01T09:00:00Z","width":4,"height":3,"bytes":100,"sidecarBytes":0,"video":false,"screenshot":false},
		{"id":"F/IMG_0002.HEIC","created":"2026-10-01T09:00:01Z","width":4,"height":3,"bytes":101,"sidecarBytes":0,"video":false,"screenshot":false},
		{"id":"F/IMG_0003.PNG","created":"2026-01-01T09:00:00Z","width":4,"height":3,"bytes":50,"sidecarBytes":0,"video":false,"screenshot":true},
		{"id":"F/IMG_0004.MOV","created":"2026-10-01T09:00:00Z","width":4,"height":3,"bytes":900,"sidecarBytes":0,"video":true,"screenshot":false}]}`,
	"analyze": `{"source":"device","analyses":[
		{"id":"F/IMG_0001.HEIC","featurePrint":"` + printA + `","aesthetics":0.1},
		{"id":"F/IMG_0002.HEIC","featurePrint":"` + printB + `","aesthetics":0.5},
		{"id":"F/IMG_0003.PNG","featurePrint":"` + printC + `","aesthetics":0}],"failed":[]}`,
}

func setSource(t *testing.T, home, source string) {
	t.Helper()
	if err := config.SetPhotosSource(config.DefaultPath(home), source); err != nil {
		t.Fatal(err)
	}
}

func scannerNamed(t *testing.T, home, name string) (metrics.Scanner, bool) {
	t.Helper()
	scans := readSnapshots(t, home)
	i := slices.IndexFunc(scans[len(scans)-1].Run.Scanners, func(s metrics.Scanner) bool { return s.Name == name })
	if i < 0 {
		return metrics.Scanner{}, false
	}
	return scans[len(scans)-1].Run.Scanners[i], true
}

func TestIPhoneScansPhotos(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	ran := fakePhotos(t, phonePhotos)
	out := run(t, "iphone")

	if !strings.Contains(out, "Photos       1 duplicate groups, 1 old screenshots") {
		t.Errorf("summary = %q", out)
	}
	if !slices.Equal(*ran, []string{"list", "analyze"}) {
		t.Errorf("ran %q", *ran)
	}
	if m, ok := scannerNamed(t, home, "iPhone photos"); !ok || m.Items != 4 || m.Error != "" {
		t.Errorf("run details: %+v, %v", m, ok)
	}
}

func TestIPhoneAsksForPhotosSourceOnce(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	fakePhotos(t, phonePhotos)
	asked := 0
	photosAsker = func(io.Writer) (string, error) { asked++; return iphone.SourceDevice, nil }
	t.Cleanup(func() { photosAsker = askInTerminal })

	run(t, "iphone", "--dry")
	if cfg, _ := config.Load(config.DefaultPath(home)); cfg.PhotosSource != "" {
		t.Errorf("--dry saved photosSource %q", cfg.PhotosSource)
	}
	run(t, "iphone")
	run(t, "iphone")
	if cfg, _ := config.Load(config.DefaultPath(home)); cfg.PhotosSource != iphone.SourceDevice {
		t.Errorf("photosSource %q", cfg.PhotosSource)
	}
	if asked != 2 {
		t.Errorf("asked %d times, want 2 (the dry run and the first real run)", asked)
	}
}

func TestIPhoneSkipsPhotosWithoutTerminal(t *testing.T) {
	fakeHome(t)
	fakePhone(t, nil)
	ran := fakePhotos(t, phonePhotos)
	out := run(t, "iphone")
	if !strings.Contains(out, "Photos weren't scanned: the iCloud Photos question needs a terminal") {
		t.Errorf("summary = %q", out)
	}
	if len(*ran) != 0 {
		t.Errorf("ran %q", *ran)
	}
}

func TestIPhonePhotosMissingHelper(t *testing.T) {
	// The real runner, with no photos-helper in the fake home's bin.
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	out := run(t, "iphone")
	if !strings.Contains(out, "photos-helper isn't installed. Run make install.") {
		t.Errorf("summary = %q", out)
	}
	if !strings.Contains(out, "iPhone       73.0 GB free") {
		t.Errorf("rest of the summary missing: %q", out)
	}
	if m, _ := scannerNamed(t, home, "iPhone photos"); m.Error != "no_helper" {
		t.Errorf("run details: %+v", m)
	}
}

func TestIPhonePhotosLocked(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	fakePhotos(t, map[string]string{"list": `{"error":"locked"}`})
	out := run(t, "iphone")
	if !strings.Contains(out, "the iPhone is locked. Unlock it") {
		t.Errorf("summary = %q", out)
	}
	if m, _ := scannerNamed(t, home, "iPhone photos"); m.Error != "locked" {
		t.Errorf("run details: %+v", m)
	}
}

func latestHTML(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "Reports/groundskeeper/latest.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIPhoneReportShowsPhotos(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	fakePhotos(t, phonePhotos)
	run(t, "iphone")
	html := latestHTML(t, home)
	for _, want := range []string{
		"4 photos and videos, read from the iPhone over USB",
		"<td>1 Oct 2026 09:00</td><td class=\"num\">2</td><td>Look alike</td>",
		"deleting the extras would free 100 B",
		"1 screenshots older than 30 days take 50 B, taken 1 Jan 2026",
		"F/IMG_0004.MOV",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if strings.Contains(html, "IMG_0001") {
		t.Error("report lists the photos in a group")
	}
}

func TestIPhoneReportShowsPhotosNote(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	fakePhotos(t, map[string]string{"list": `{"error":"locked"}`})
	run(t, "iphone")
	if html := latestHTML(t, home); !strings.Contains(html, "Photos weren&#39;t scanned: the iPhone is locked.") {
		t.Errorf("report lacks the note")
	}
}

func TestIPhoneReportEscapesPhotoIDs(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	setSource(t, home, iphone.SourceDevice)
	answers := maps.Clone(phonePhotos)
	answers["list"] = strings.ReplaceAll(answers["list"], "F/IMG_0004.MOV", `F/<script>alert(1)</script>.MOV`)
	fakePhotos(t, answers)
	run(t, "iphone")
	html := latestHTML(t, home)
	if strings.Contains(html, "<script>alert(1)") || !strings.Contains(html, "&lt;script&gt;alert(1)") {
		t.Error("photo ID not escaped")
	}
}
