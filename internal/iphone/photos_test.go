package iphone

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"slices"
	"testing"
	"time"
)

// fakeHelper answers every command with a testdata/helper file and records
// what it was asked.
type fakeHelper struct {
	file  string
	err   error
	args  []string
	stdin []byte
}

func (f *fakeHelper) run(_ context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	f.args = append([]string{name}, args...)
	f.stdin = stdin
	data, err := os.ReadFile("testdata/helper/" + f.file)
	if err != nil {
		return nil, err
	}
	return data, f.err
}

func (f *fakeHelper) helper(source string) Helper {
	return Helper{Path: "/bin/photos-helper", Source: source, Run: f.run}
}

func TestListDevice(t *testing.T) {
	f := &fakeHelper{file: "list-device.json"}
	assets, err := f.helper(SourceDevice).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/bin/photos-helper", "list", "--source", "device"}
	if !slices.Equal(f.args, want) {
		t.Errorf("args %q, want %q", f.args, want)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(assets))
	}
	a := assets[1]
	if a.ID != "202610__/IMG_0002.HEIC" || a.Burst != "B1" || a.Bytes != 2_050_000 || a.Width != 4032 ||
		!a.Created.Equal(time.Date(2026, 10, 1, 9, 0, 2, 0, time.UTC)) {
		t.Errorf("got %+v", a)
	}
	if assets[0].SidecarBytes != 1_500_000 || assets[0].Favorite {
		t.Errorf("got %+v", assets[0])
	}
}

func TestListLibrary(t *testing.T) {
	f := &fakeHelper{file: "list-library.json"}
	assets, err := f.helper(SourceMacLibrary).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.args[3] != "mac-library" {
		t.Errorf("source %q", f.args[3])
	}
	if a := assets[0]; !a.Edited || !a.InAlbum || a.Favorite {
		t.Errorf("got %+v", a)
	}
}

func TestAnalyze(t *testing.T) {
	f := &fakeHelper{file: "analyze-device.json"}
	ids := []string{"202610__/IMG_0001.HEIC", "202610__/IMG_9999.HEIC"}
	analyses, failed, err := f.helper(SourceDevice).Analyze(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if string(f.stdin) != `["202610__/IMG_0001.HEIC","202610__/IMG_9999.HEIC"]` {
		t.Errorf("stdin %s", f.stdin)
	}
	if len(analyses) != 1 || analyses[0].Aesthetics != 0.27 || len(analyses[0].FeaturePrint) != 12 {
		t.Errorf("got %+v", analyses)
	}
	if len(failed) != 1 || failed[0].Reason != "not-found" {
		t.Errorf("got %+v", failed)
	}
}

func TestHelperErrorKind(t *testing.T) {
	f := &fakeHelper{file: "error-locked.json", err: errors.New("exit status 1")}
	_, err := f.helper(SourceDevice).List(context.Background())
	if !errors.Is(err, ErrLocked) {
		t.Errorf("got %v, want ErrLocked", err)
	}
}

func TestHelperFailedWithoutAnswer(t *testing.T) {
	f := &fakeHelper{file: "list-device.json", err: errors.New("signal: killed")}
	_, err := f.helper(SourceDevice).List(context.Background())
	if !errors.Is(err, ErrHelper) {
		t.Errorf("got %v, want ErrHelper", err)
	}
}

func floats(v ...float32) []byte {
	b := make([]byte, 0, 4*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
	}
	return b
}

func TestDistance(t *testing.T) {
	if d := Distance(floats(1, 2, 0.5), floats(0, 0, 0.5)); d != 5 {
		t.Errorf("got %v, want 5", d)
	}
	if d := Distance(floats(1), floats(1, 2)); !math.IsInf(d, 1) {
		t.Errorf("got %v, want +Inf for different lengths", d)
	}
}
