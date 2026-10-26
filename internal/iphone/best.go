package iphone

import "math"

// Protected reports whether an asset is always kept: a favorite, an edited
// photo or one in an album. These are only known for the Mac library.
func (a Asset) Protected() bool { return a.Favorite || a.Edited || a.InAlbum }

// Keep returns which of a group's IDs to keep, in the group's order: every
// protected asset, plus the best shot. The best shot has the highest
// aesthetics score, chosen among the protected assets when there are any,
// so a favorite isn't kept alongside an extra photo. A missing score ranks
// lowest, and ties go to the earlier ID, the older photo.
func Keep(ids []string, assets map[string]Asset, aesthetics map[string]float64) []string {
	anyProtected := false
	for _, id := range ids {
		anyProtected = anyProtected || assets[id].Protected()
	}
	best, bestScore := "", math.Inf(-1)
	for _, id := range ids {
		if anyProtected && !assets[id].Protected() {
			continue
		}
		score, ok := aesthetics[id]
		if !ok {
			score = math.Inf(-1)
		}
		if best == "" || score > bestScore {
			best, bestScore = id, score
		}
	}
	var keep []string
	for _, id := range ids {
		if id == best || assets[id].Protected() {
			keep = append(keep, id)
		}
	}
	return keep
}
