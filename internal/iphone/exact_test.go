package iphone

import (
	"slices"
	"testing"
	"time"
)

func sized(id string, minutes int, bytes int64, width int) Asset {
	return Asset{ID: id, Created: t0.Add(time.Duration(minutes) * time.Minute), Bytes: bytes, Width: width, Height: 100}
}

func TestExactCandidates(t *testing.T) {
	assets := []Asset{
		sized("a", 0, 500, 200),
		sized("b", 1, 500, 200), // same as a
		sized("c", 2, 500, 300), // same size, other dimensions
		sized("d", 3, 0, 200),   // no size
		sized("e", 4, 0, 200),   // no size
		sized("f", 5, 700, 200),
	}
	got := ExactCandidates(assets)
	if want := []string{"a", "b"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExactDuplicates(t *testing.T) {
	assets := []Asset{
		sized("late", 90, 500, 200),
		sized("a", 0, 500, 200),
		sized("b", 1, 500, 200), // same shape as a, different contents
		sized("c", 2, 500, 200), // same shape, hash failed
		sized("x", 5, 800, 200),
		sized("y", 6, 800, 200),
	}
	hashes := []FileHash{{"a", "h1"}, {"late", "h1"}, {"b", "h2"}, {"x", "h3"}, {"y", "h3"}}
	got := ExactDuplicates(assets, hashes)
	want := [][]string{{"a", "late"}, {"x", "y"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExactDuplicatesNeedsSameShape(t *testing.T) {
	// The same hash with a different size can only be a stale or bad answer.
	assets := []Asset{sized("a", 0, 500, 200), sized("b", 1, 501, 200)}
	if got := ExactDuplicates(assets, []FileHash{{"a", "h1"}, {"b", "h1"}}); len(got) != 0 {
		t.Errorf("got %q, want no groups", got)
	}
}
