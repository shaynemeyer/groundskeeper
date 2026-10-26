package clean

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

var reviewed = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

var scanned = []iphone.Asset{
	{ID: "F/IMG_0001.HEIC", Created: reviewed.Add(-time.Hour), Bytes: 100, SidecarBytes: 20},
	{ID: "F/IMG_0002.HEIC", Created: reviewed.Add(-time.Hour), Bytes: 200},
	{ID: "F/IMG_0003.HEIC", Edited: true, Bytes: 300},
	{ID: "L/0004", Favorite: true, Bytes: 400},
	{ID: "L/0005", InAlbum: true, Bytes: 500},
}

func TestNewSelection(t *testing.T) {
	s, err := NewSelection(reviewed, iphone.SourceDevice, scanned, []string{"F/IMG_0001.HEIC", "F/IMG_0002.HEIC"})
	if err != nil {
		t.Fatal(err)
	}
	want := Approved{ID: "F/IMG_0001.HEIC", Created: reviewed.Add(-time.Hour), Bytes: 100, SidecarBytes: 20}
	if len(s.Delete) != 2 || s.Delete[0] != want || s.Source != iphone.SourceDevice || !s.Time.Equal(reviewed) {
		t.Errorf("got %+v", s)
	}
	if s.Bytes() != 320 {
		t.Errorf("bytes %d, want 320", s.Bytes())
	}
}

func TestNewSelectionRefusesProtected(t *testing.T) {
	for _, id := range []string{"F/IMG_0003.HEIC", "L/0004", "L/0005"} {
		if _, err := NewSelection(reviewed, iphone.SourceDevice, scanned, []string{"F/IMG_0001.HEIC", id}); err == nil {
			t.Errorf("%s: want refusal", id)
		}
	}
}

func TestNewSelectionRefusesUnknownID(t *testing.T) {
	if _, err := NewSelection(reviewed, iphone.SourceDevice, scanned, []string{"F/IMG_9999.HEIC"}); err == nil {
		t.Error("want refusal")
	}
}

func TestSelectionRoundTrip(t *testing.T) {
	home := t.TempDir()
	path := SelectionPath(home)
	if _, err := LoadSelection(path); !errors.Is(err, ErrNoSelection) {
		t.Fatalf("before saving: %v", err)
	}
	s, err := NewSelection(reviewed, iphone.SourceDevice, scanned, []string{"F/IMG_0001.HEIC"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSelection(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSelection(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Delete) != 1 || got.Delete[0] != s.Delete[0] || got.Source != s.Source {
		t.Errorf("got %+v, want %+v", got, s)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v, want 0700", info.Mode().Perm())
	}
}

func TestSelectionSurvivesPruning(t *testing.T) {
	home := t.TempDir()
	path := SelectionPath(home)
	if err := SaveSelection(path, Selection{Time: reviewed.AddDate(-1, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Prune(history.Dir(home), reviewed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("pruning removed the selection: %v", err)
	}
}
