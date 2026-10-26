package iphone

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

type listAnswer struct {
	Source string  `json:"source"`
	Assets []Asset `json:"assets"`
}

func TestScanPhotos(t *testing.T) {
	assets := []Asset{
		sized("a", 0, 10, 4), sized("b", 1, 11, 4),
		{ID: "shot", Screenshot: true, Created: now.AddDate(0, 0, -40), Bytes: 5},
		{ID: "clip", Video: true, Created: t0, Bytes: 90},
	}
	s := &scripted{answers: map[string]any{
		"list":    listAnswer{Source: SourceDevice, Assets: assets},
		"analyze": analyzeAnswer{Analyses: []Analysis{analysis("a", 0, 0.1), analysis("b", 0.1, 0.4), analysis("shot", 7, 0)}},
	}}
	h := Helper{Source: SourceDevice, Run: s.run}
	got, err := ScanPhotos(context.Background(), h, now, 10*time.Minute, 30, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != SourceDevice || len(got.Assets) != 4 {
		t.Errorf("source %q, %d assets", got.Source, len(got.Assets))
	}
	if len(got.Duplicates.Groups) != 1 || !slices.Equal(got.Duplicates.Groups[0].IDs, []string{"a", "b"}) {
		t.Errorf("groups %+v", got.Duplicates.Groups)
	}
	if !slices.Equal(got.OldScreenshots, []string{"shot"}) || !slices.Equal(got.LargestVideos, []string{"clip"}) {
		t.Errorf("screenshots %q, videos %q", got.OldScreenshots, got.LargestVideos)
	}
}

func TestScanPhotosListError(t *testing.T) {
	run := func(context.Context, []byte, string, ...string) ([]byte, error) {
		return []byte(`{"error":"no-device"}`), errors.New("exit status 1")
	}
	if _, err := ScanPhotos(context.Background(), Helper{Run: run}, now, time.Minute, 30, 5); !errors.Is(err, ErrNoDevice) {
		t.Errorf("got %v, want ErrNoDevice", err)
	}
}
