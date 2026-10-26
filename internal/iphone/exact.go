package iphone

import (
	"slices"
)

// shape is what two files must share before they're worth hashing.
type shape struct {
	bytes         int64
	width, height int
}

func shapeOf(a Asset) shape { return shape{a.Bytes, a.Width, a.Height} }

// ExactCandidates returns the IDs of assets that share their size and
// dimensions with another asset, in the order given. Only these are hashed,
// because the device path reads every hashed file over USB. Assets with no
// size are left out, so a missing size never matches.
func ExactCandidates(assets []Asset) []string {
	count := map[shape]int{}
	for _, a := range assets {
		count[shapeOf(a)]++
	}
	var ids []string
	for _, a := range assets {
		if a.Bytes > 0 && count[shapeOf(a)] > 1 {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// ExactDuplicates groups assets whose size, dimensions and SHA-256 all match.
// Assets without a hash are left out. Each group holds two or more IDs,
// oldest first, and groups are ordered by their oldest photo.
func ExactDuplicates(assets []Asset, hashes []FileHash) [][]string {
	sums := map[string]string{}
	for _, h := range hashes {
		sums[h.ID] = h.SHA256
	}
	type key struct {
		shape
		sum string
	}
	sorted := slices.Clone(assets)
	slices.SortStableFunc(sorted, func(a, b Asset) int { return a.Created.Compare(b.Created) })
	byKey := map[key][]string{}
	var order []key
	for _, a := range sorted {
		sum := sums[a.ID]
		if sum == "" || a.Bytes == 0 {
			continue
		}
		k := key{shapeOf(a), sum}
		if byKey[k] == nil {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], a.ID)
	}
	var groups [][]string
	for _, k := range order {
		if len(byKey[k]) > 1 {
			groups = append(groups, byKey[k])
		}
	}
	return groups
}
