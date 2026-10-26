package iphone

import (
	"slices"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// photo is an asset taken minutes after t0, with a one-value feature print.
func photo(id string, minutes float64, print float32) (Asset, Analysis) {
	created := t0.Add(time.Duration(minutes * float64(time.Minute)))
	return Asset{ID: id, Created: created}, Analysis{ID: id, FeaturePrint: floats(print)}
}

func group(t *testing.T, assets []Asset, analyses []Analysis) [][]string {
	t.Helper()
	return NearDuplicates(assets, analyses, 10*time.Minute, 0.25)
}

func collect(pairs ...func() (Asset, Analysis)) ([]Asset, []Analysis) {
	var assets []Asset
	var analyses []Analysis
	for _, p := range pairs {
		a, b := p()
		assets = append(assets, a)
		analyses = append(analyses, b)
	}
	return assets, analyses
}

func p(id string, minutes float64, print float32) func() (Asset, Analysis) {
	return func() (Asset, Analysis) { return photo(id, minutes, print) }
}

func TestNearDuplicatesWindow(t *testing.T) {
	// b is close to a in time and look; c looks the same but is 30 minutes
	// later; d is close in time but looks different.
	assets, analyses := collect(p("c", 30, 0.1), p("a", 0, 0), p("b", 5, 0.4), p("d", 6, 3))
	got := group(t, assets, analyses)
	want := [][]string{{"a", "b"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNearDuplicatesBurst(t *testing.T) {
	assets, analyses := collect(p("a", 0, 0), p("b", 45, 0.1))
	assets[0].Burst, assets[1].Burst = "B1", "B1"
	got := group(t, assets, analyses)
	if want := [][]string{{"a", "b"}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNearDuplicatesChain(t *testing.T) {
	// a-b and b-c are each 0.16 apart; a-c is 0.64, but they chain through b.
	// e and f form a later group.
	assets, analyses := collect(p("a", 0, 0), p("b", 1, 0.4), p("c", 2, 0.8), p("e", 60, 5), p("f", 61, 5.1))
	got := group(t, assets, analyses)
	want := [][]string{{"a", "b", "c"}, {"e", "f"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNearDuplicatesSkipsUnanalyzedAndUndated(t *testing.T) {
	assets, analyses := collect(p("a", 0, 0), p("b", 1, 0), p("c", 2, 0))
	analyses = analyses[1:]         // a failed analysis
	assets[2].Created = time.Time{} // c has no date and no burst
	if got := group(t, assets, analyses); len(got) != 0 {
		t.Errorf("got %q, want no groups", got)
	}
}
