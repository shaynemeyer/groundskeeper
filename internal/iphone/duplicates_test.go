package iphone

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"
)

// scripted answers each photos-helper command with a value as JSON and
// records the IDs each command was given.
type scripted struct {
	answers map[string]any
	asked   map[string][]string
}

func (s *scripted) run(_ context.Context, stdin []byte, _ string, args ...string) ([]byte, error) {
	var ids []string
	if len(stdin) > 0 {
		if err := json.Unmarshal(stdin, &ids); err != nil {
			return nil, err
		}
	}
	if s.asked == nil {
		s.asked = map[string][]string{}
	}
	s.asked[args[0]] = ids
	return json.Marshal(s.answers[args[0]])
}

func (s *scripted) find(t *testing.T, assets []Asset) Duplicates {
	t.Helper()
	found, err := FindDuplicates(context.Background(), Helper{Source: SourceDevice, Run: s.run}, assets, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

type analyzeAnswer struct {
	Analyses []Analysis `json:"analyses"`
	Failed   []Failure  `json:"failed"`
}

type hashAnswer struct {
	Hashes []FileHash `json:"hashes"`
	Failed []Failure  `json:"failed"`
}

func analysis(id string, print float32, aesthetics float64) Analysis {
	return Analysis{ID: id, FeaturePrint: floats(print), Aesthetics: aesthetics}
}

func TestFindDuplicates(t *testing.T) {
	assets := []Asset{
		sized("a", 0, 10, 4), sized("b", 1, 11, 4),
		sized("c", 100, 50, 8), sized("d", 200, 50, 8),
		sized("e", 300, 70, 8),
		{ID: "v", Video: true, Created: t0, Bytes: 90},
	}
	s := &scripted{answers: map[string]any{
		"analyze": analyzeAnswer{
			Analyses: []Analysis{analysis("a", 0, 0.1), analysis("b", 0.1, 0.4), analysis("c", 5, 0), analysis("d", 9, 0)},
			Failed:   []Failure{{ID: "e", Reason: "thumbnail"}},
		},
		"hash": hashAnswer{Hashes: []FileHash{{ID: "c", SHA256: "aa"}, {ID: "d", SHA256: "aa"}}},
	}}
	found := s.find(t, assets)

	if want := []string{"a", "b", "c", "d", "e"}; !slices.Equal(s.asked["analyze"], want) {
		t.Errorf("analyzed %q, want %q", s.asked["analyze"], want)
	}
	if want := []string{"c", "d"}; !slices.Equal(s.asked["hash"], want) {
		t.Errorf("hashed %q, want %q", s.asked["hash"], want)
	}
	want := []Group{
		{IDs: []string{"a", "b"}, Keep: []string{"b"}},
		{IDs: []string{"c", "d"}, Keep: []string{"c"}, Exact: true},
	}
	if !slices.EqualFunc(found.Groups, want, equalGroup) {
		t.Errorf("got %+v, want %+v", found.Groups, want)
	}
	if len(found.Failed) != 1 || found.Failed[0].ID != "e" {
		t.Errorf("failed %+v", found.Failed)
	}
}

func TestFindDuplicatesMergesExactAndNear(t *testing.T) {
	// x and y look alike; y and z are byte-identical but too far apart in
	// time to be compared by look. They make one group, not exact.
	assets := []Asset{sized("x", 0, 10, 4), sized("y", 1, 50, 8), sized("z", 600, 50, 8)}
	s := &scripted{answers: map[string]any{
		"analyze": analyzeAnswer{Analyses: []Analysis{analysis("x", 0, 0), analysis("y", 0.1, 0), analysis("z", 0.1, 0)}},
		"hash":    hashAnswer{Hashes: []FileHash{{ID: "y", SHA256: "bb"}, {ID: "z", SHA256: "bb"}}},
	}}
	found := s.find(t, assets)
	want := []Group{{IDs: []string{"x", "y", "z"}, Keep: []string{"x"}}}
	if !slices.EqualFunc(found.Groups, want, equalGroup) {
		t.Errorf("got %+v, want %+v", found.Groups, want)
	}
}

func TestFindDuplicatesHashesOnlyCandidates(t *testing.T) {
	assets := []Asset{sized("a", 0, 10, 4), sized("b", 1, 11, 4)}
	s := &scripted{answers: map[string]any{"analyze": analyzeAnswer{}}}
	if found := s.find(t, assets); len(found.Groups) != 0 {
		t.Errorf("got %+v", found.Groups)
	}
	if _, asked := s.asked["hash"]; asked {
		t.Error("hash ran with no candidates")
	}
}

func TestFindDuplicatesHelperError(t *testing.T) {
	run := func(context.Context, []byte, string, ...string) ([]byte, error) {
		return []byte(`{"error":"locked"}`), errors.New("exit status 1")
	}
	_, err := FindDuplicates(context.Background(), Helper{Run: run}, []Asset{sized("a", 0, 10, 4)}, time.Minute)
	if !errors.Is(err, ErrLocked) {
		t.Errorf("got %v, want ErrLocked", err)
	}
}

func equalGroup(a, b Group) bool {
	return slices.Equal(a.IDs, b.IDs) && slices.Equal(a.Keep, b.Keep) && a.Exact == b.Exact
}
