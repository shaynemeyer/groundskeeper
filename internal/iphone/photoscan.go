package iphone

import (
	"context"
	"time"
)

// Photos is what a photo scan found in one source.
type Photos struct {
	Source         string     `json:"source"`
	Assets         []Asset    `json:"assets"`
	Duplicates     Duplicates `json:"duplicates"`
	OldScreenshots []string   `json:"oldScreenshots"`
	LargestVideos  []string   `json:"largestVideos"`
}

// ScanPhotos lists every asset in the helper's source, then finds duplicate
// groups, screenshots taken more than screenshotDays before now, and the
// largest videos, at most videos of them.
func ScanPhotos(ctx context.Context, h Helper, now time.Time, window time.Duration, screenshotDays, videos int) (Photos, error) {
	assets, err := h.List(ctx)
	if err != nil {
		return Photos{}, err
	}
	dups, err := FindDuplicates(ctx, h, assets, window)
	if err != nil {
		return Photos{}, err
	}
	return Photos{
		Source:         h.Source,
		Assets:         assets,
		Duplicates:     dups,
		OldScreenshots: OldScreenshots(assets, now, screenshotDays),
		LargestVideos:  LargestVideos(assets, videos),
	}, nil
}
