package iphone

import (
	"slices"
	"testing"
)

func keep(t *testing.T, ids []string, assets []Asset, scores map[string]float64) []string {
	t.Helper()
	byID := map[string]Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	return Keep(ids, byID, scores)
}

func TestKeepBestScore(t *testing.T) {
	assets := []Asset{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := keep(t, []string{"a", "b", "c"}, assets, map[string]float64{"a": 0.1, "b": 0.5, "c": -0.2})
	if want := []string{"b"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKeepProtected(t *testing.T) {
	// b scores best, but a and c are protected, so the best shot comes from
	// them and b isn't kept.
	assets := []Asset{{ID: "a", Favorite: true}, {ID: "b"}, {ID: "c", InAlbum: true}}
	got := keep(t, []string{"a", "b", "c"}, assets, map[string]float64{"a": 0.1, "b": 0.9, "c": 0.3})
	if want := []string{"a", "c"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKeepEditedOverBetterScore(t *testing.T) {
	assets := []Asset{{ID: "a"}, {ID: "b", Edited: true}}
	got := keep(t, []string{"a", "b"}, assets, map[string]float64{"a": 0.9, "b": -0.5})
	if want := []string{"b"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKeepTiesAndMissingScores(t *testing.T) {
	assets := []Asset{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	if got := keep(t, []string{"a", "b"}, assets, map[string]float64{"a": 0.2, "b": 0.2}); !slices.Equal(got, []string{"a"}) {
		t.Errorf("tie: got %q, want the older a", got)
	}
	if got := keep(t, []string{"a", "b", "c"}, assets, map[string]float64{"c": -0.9}); !slices.Equal(got, []string{"c"}) {
		t.Errorf("missing: got %q, want c, the only one scored", got)
	}
	if got := keep(t, []string{"a", "b"}, assets, nil); !slices.Equal(got, []string{"a"}) {
		t.Errorf("no scores: got %q, want the older a", got)
	}
}
