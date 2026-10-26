package iphone

import (
	"cmp"
	"slices"
	"time"
)

// OldScreenshots returns the IDs of screenshots taken more than days before
// now, oldest first. Protected screenshots are left out, and so are undated
// ones, whose age isn't known.
func OldScreenshots(assets []Asset, now time.Time, days int) []string {
	cutoff := now.AddDate(0, 0, -days)
	var old []Asset
	for _, a := range assets {
		if a.Screenshot && !a.Protected() && !a.Created.IsZero() && a.Created.Before(cutoff) {
			old = append(old, a)
		}
	}
	slices.SortStableFunc(old, func(a, b Asset) int { return a.Created.Compare(b.Created) })
	return ids(old)
}

// LargestVideos returns the IDs of the n largest videos, counting their
// sidecar files, largest first with ties going to the older video. Protected
// videos are left out.
func LargestVideos(assets []Asset, n int) []string {
	var videos []Asset
	for _, a := range assets {
		if a.Video && !a.Protected() {
			videos = append(videos, a)
		}
	}
	slices.SortStableFunc(videos, func(a, b Asset) int {
		return cmp.Or(cmp.Compare(b.Bytes+b.SidecarBytes, a.Bytes+a.SidecarBytes), a.Created.Compare(b.Created))
	})
	return ids(videos[:min(n, len(videos))])
}

func ids(assets []Asset) []string {
	out := make([]string, len(assets))
	for i, a := range assets {
		out[i] = a.ID
	}
	return out
}
