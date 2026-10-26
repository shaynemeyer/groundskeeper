package iphone

import (
	"context"
	"slices"
	"time"
)

// Group is a set of duplicate assets, oldest first, with the ones to keep.
type Group struct {
	IDs  []string
	Keep []string
	// Exact means every asset is a byte-identical copy of the others.
	Exact bool
}

// Duplicates holds the groups found, and the assets analyze or hash failed on.
type Duplicates struct {
	Groups []Group
	Failed []Failure
}

// FindDuplicates analyzes the photos among assets (videos aren't compared by
// look), hashes the assets that share their size and dimensions with
// another, and groups near and exact duplicates. Groups that share an asset
// are merged into one, and each group's keepers come from Keep.
func FindDuplicates(ctx context.Context, h Helper, assets []Asset, window time.Duration) (Duplicates, error) {
	var found Duplicates
	var photos []string
	for _, a := range assets {
		if !a.Video {
			photos = append(photos, a.ID)
		}
	}
	var analyses []Analysis
	if len(photos) > 0 {
		var failed []Failure
		var err error
		if analyses, failed, err = h.Analyze(ctx, photos); err != nil {
			return Duplicates{}, err
		}
		found.Failed = append(found.Failed, failed...)
	}
	var hashes []FileHash
	if candidates := ExactCandidates(assets); len(candidates) > 0 {
		var failed []Failure
		var err error
		if hashes, failed, err = h.Hash(ctx, candidates); err != nil {
			return Duplicates{}, err
		}
		found.Failed = append(found.Failed, failed...)
	}

	byID := map[string]Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	scores := map[string]float64{}
	for _, a := range analyses {
		scores[a.ID] = a.Aesthetics
	}
	sums := map[string]string{}
	for _, s := range hashes {
		sums[s.ID] = s.SHA256
	}
	near := NearDuplicates(assets, analyses, window, NearDistance)
	exact := ExactDuplicates(assets, hashes)
	for _, ids := range merge(assets, near, exact) {
		found.Groups = append(found.Groups, Group{IDs: ids, Keep: Keep(ids, byID, scores), Exact: sameHash(ids, sums)})
	}
	return found, nil
}

// merge joins groups that share an asset. Each merged group holds its IDs
// oldest first, and groups are ordered by their oldest asset.
func merge(assets []Asset, groupings ...[][]string) [][]string {
	sorted := slices.Clone(assets)
	slices.SortStableFunc(sorted, func(a, b Asset) int { return a.Created.Compare(b.Created) })
	index := map[string]int{}
	for i, a := range sorted {
		index[a.ID] = i
	}
	links := newUnion(len(sorted))
	for _, groups := range groupings {
		for _, ids := range groups {
			for _, id := range ids[1:] {
				links.join(index[ids[0]], index[id])
			}
		}
	}
	return links.groups(func(i int) string { return sorted[i].ID })
}

// sameHash reports whether every ID has the same known SHA-256.
func sameHash(ids []string, sums map[string]string) bool {
	for _, id := range ids {
		if sums[id] == "" || sums[id] != sums[ids[0]] {
			return false
		}
	}
	return true
}
