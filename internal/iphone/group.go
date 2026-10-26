package iphone

import (
	"cmp"
	"slices"
	"time"
)

// NearDistance is the feature-print distance up to which two photos count as
// near-duplicates. On one phone's 1,158 photos, every pair the user judged
// at 0.0226 or less was a duplicate, while between 0.026 and 0.048 answers
// were mixed (5 duplicates, 9 not). Choosing 0.025 left 5 of the 17
// duplicates ungrouped but grouped none of the 12 non-duplicates.
const NearDistance = 0.025

// NearDuplicates links photos that look alike into groups. Two photos are
// compared when they were taken within window of each other or are in the
// same burst, and linked when their feature-print distance is at most
// maxDistance. Links chain, so a group can hold photos further apart than
// maxDistance through ones in between. Assets without an analysis are left
// out. Each group holds two or more IDs, oldest first, and groups are
// ordered by their oldest photo.
func NearDuplicates(assets []Asset, analyses []Analysis, window time.Duration, maxDistance float64) [][]string {
	prints := map[string][]byte{}
	for _, a := range analyses {
		prints[a.ID] = a.FeaturePrint
	}
	var photos []Asset
	for _, a := range assets {
		if prints[a.ID] != nil {
			photos = append(photos, a)
		}
	}
	slices.SortStableFunc(photos, func(a, b Asset) int { return a.Created.Compare(b.Created) })

	links := newUnion(len(photos))
	link := func(i, j int) {
		if Distance(prints[photos[i].ID], prints[photos[j].ID]) <= maxDistance {
			links.join(i, j)
		}
	}
	for i := range photos {
		for j := i + 1; j < len(photos) && withinWindow(photos[i], photos[j], window); j++ {
			link(i, j)
		}
	}
	bursts := map[string][]int{}
	for i, p := range photos {
		if p.Burst != "" {
			bursts[p.Burst] = append(bursts[p.Burst], i)
		}
	}
	for _, members := range bursts {
		for x, i := range members {
			for _, j := range members[x+1:] {
				link(i, j)
			}
		}
	}
	return links.groups(func(i int) string { return photos[i].ID })
}

// withinWindow reports whether b, taken no earlier than a, was taken within
// window of a. Photos without a date are only compared within their burst.
func withinWindow(a, b Asset, window time.Duration) bool {
	if a.Created.IsZero() || b.Created.IsZero() {
		return false
	}
	return b.Created.Sub(a.Created) <= window
}

// union is a disjoint-set forest over indexes 0 to n-1.
type union []int

func newUnion(n int) union {
	u := make(union, n)
	for i := range u {
		u[i] = i
	}
	return u
}

func (u union) find(i int) int {
	for u[i] != i {
		u[i] = u[u[i]]
		i = u[i]
	}
	return i
}

func (u union) join(i, j int) {
	u[u.find(j)] = u.find(i)
}

// groups returns the sets with two or more members, named by name, each in
// index order and ordered by their first index.
func (u union) groups(name func(int) string) [][]string {
	byRoot := map[int][]int{}
	for i := range u {
		r := u.find(i)
		byRoot[r] = append(byRoot[r], i)
	}
	var sets [][]int
	for _, members := range byRoot {
		if len(members) > 1 {
			sets = append(sets, members)
		}
	}
	slices.SortFunc(sets, func(a, b []int) int { return cmp.Compare(a[0], b[0]) })
	out := make([][]string, len(sets))
	for k, members := range sets {
		for _, i := range members {
			out[k] = append(out[k], name(i))
		}
	}
	return out
}
