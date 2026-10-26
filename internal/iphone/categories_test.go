package iphone

import (
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func daysAgo(d int) time.Time { return now.AddDate(0, 0, -d) }

func TestOldScreenshots(t *testing.T) {
	assets := []Asset{
		{ID: "new", Screenshot: true, Created: daysAgo(10)},
		{ID: "older", Screenshot: true, Created: daysAgo(90)},
		{ID: "old", Screenshot: true, Created: daysAgo(31)},
		{ID: "photo", Created: daysAgo(90)},
		{ID: "edge", Screenshot: true, Created: daysAgo(30)},
	}
	got := OldScreenshots(assets, now, 30)
	if want := []string{"older", "old"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOldScreenshotsSkipsProtectedAndUndated(t *testing.T) {
	assets := []Asset{
		{ID: "edited", Screenshot: true, Edited: true, Created: daysAgo(90)},
		{ID: "favorite", Screenshot: true, Favorite: true, Created: daysAgo(90)},
		{ID: "album", Screenshot: true, InAlbum: true, Created: daysAgo(90)},
		{ID: "undated", Screenshot: true},
	}
	if got := OldScreenshots(assets, now, 30); len(got) != 0 {
		t.Errorf("got %q, want none", got)
	}
}

func TestLargestVideos(t *testing.T) {
	assets := []Asset{
		{ID: "small", Video: true, Bytes: 10},
		{ID: "big", Video: true, Bytes: 100},
		{ID: "photo", Bytes: 1000},
		{ID: "sidecars", Video: true, Bytes: 40, SidecarBytes: 70},
		{ID: "mid", Video: true, Bytes: 50},
	}
	got := LargestVideos(assets, 3)
	if want := []string{"sidecars", "big", "mid"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := LargestVideos(assets, 10); len(got) != 4 {
		t.Errorf("got %q, want all 4 videos", got)
	}
}

func TestLargestVideosTiesAndProtected(t *testing.T) {
	assets := []Asset{
		{ID: "newer", Video: true, Bytes: 50, Created: daysAgo(1)},
		{ID: "older", Video: true, Bytes: 50, Created: daysAgo(5)},
		{ID: "favorite", Video: true, Favorite: true, Bytes: 500},
	}
	got := LargestVideos(assets, 5)
	if want := []string{"older", "newer"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
