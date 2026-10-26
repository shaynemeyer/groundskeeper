package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

// saveSelection saves a review approving two photos from source.
func saveSelection(t *testing.T, home, source string) {
	t.Helper()
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	sel := clean.Selection{Time: scanTime, Source: source, Delete: []clean.Approved{
		{ID: "F/IMG_0001.HEIC", Created: created, Bytes: 1000},
		{ID: "F/IMG_0002.HEIC", Created: created, Bytes: 2000},
	}}
	if err := clean.SaveSelection(clean.SelectionPath(home), sel); err != nil {
		t.Fatal(err)
	}
}

// fakeDelete answers photos-helper delete with answer and records each
// call's arguments and stdin.
type fakeDelete struct {
	calls  [][]string
	stdin  []string
	answer string
}

func (f *fakeDelete) install(t *testing.T) {
	t.Helper()
	photoRunner = func(_ context.Context, in []byte, _ string, args ...string) ([]byte, error) {
		f.calls = append(f.calls, args)
		f.stdin = append(f.stdin, string(in))
		return []byte(f.answer), nil
	}
	t.Cleanup(func() { photoRunner = iphone.RunHelper })
}

// remaining returns the IDs left in the saved review.
func remaining(t *testing.T, home string) []string {
	t.Helper()
	sel, err := clean.LoadSelection(clean.SelectionPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range sel.Delete {
		ids = append(ids, a.ID)
	}
	return ids
}

const oneDeleted = `{"source":"device","deleted":["F/IMG_0001.HEIC"],"failed":[{"id":"F/IMG_0002.HEIC","reason":"mismatch"}]}`

func TestCleanPhotosNeedsReview(t *testing.T) {
	fakeHome(t)
	if err := execute("clean", "photos"); !errors.Is(err, clean.ErrNoSelection) || !strings.Contains(err.Error(), "gk review photos") {
		t.Errorf("err = %v", err)
	}
}

func TestCleanPhotosDryRun(t *testing.T) {
	home := fakeHome(t)
	saveSelection(t, home, iphone.SourceDevice)
	f := &fakeDelete{answer: oneDeleted}
	f.install(t)
	out := run(t, "clean", "photos")
	if !strings.Contains(out, "2 photos, 3.0 kB") || !strings.Contains(out, "from the iPhone") || !strings.Contains(out, "Dry run") {
		t.Errorf("output %q", out)
	}
	if len(f.calls) != 0 {
		t.Errorf("dry run called the helper: %q", f.calls)
	}
}

func TestCleanPhotosDeviceNeedsTypedCount(t *testing.T) {
	for _, input := range []string{"y\n", "yes\n", "1\n", "\n"} {
		home := fakeHome(t)
		saveSelection(t, home, iphone.SourceDevice)
		f := &fakeDelete{answer: oneDeleted}
		f.install(t)
		out := runWithInput(t, input, "clean", "photos", "--apply")
		if len(f.calls) != 0 || !strings.Contains(out, "Nothing was deleted.") || !strings.Contains(out, "permanent") {
			t.Errorf("answer %q deleted: calls %q, output %q", input, f.calls, out)
		}
	}
}

func TestCleanPhotosDeviceDeletes(t *testing.T) {
	home := fakeHome(t)
	saveSelection(t, home, iphone.SourceDevice)
	f := &fakeDelete{answer: oneDeleted}
	f.install(t)
	out := runWithInput(t, "2\n", "clean", "photos", "--apply")
	if len(f.calls) != 1 || f.calls[0][0] != "delete" || f.calls[0][2] != iphone.SourceDevice {
		t.Fatalf("calls %q", f.calls)
	}
	want := `[{"id":"F/IMG_0001.HEIC","bytes":1000,"created":"2026-10-01T09:00:00Z"},{"id":"F/IMG_0002.HEIC","bytes":2000,"created":"2026-10-01T09:00:00Z"}]`
	if f.stdin[0] != want {
		t.Errorf("stdin %s", f.stdin[0])
	}
	for _, s := range []string{"Deleted 1 photos, freeing about 1.0 kB.", "F/IMG_0002.HEIC: changed since the scan"} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
	if ids := remaining(t, home); len(ids) != 1 || ids[0] != "F/IMG_0002.HEIC" {
		t.Errorf("review now holds %q, want only the photo that wasn't deleted", ids)
	}
}

func TestCleanPhotosLibraryAsksYesNo(t *testing.T) {
	home := fakeHome(t)
	saveSelection(t, home, iphone.SourceMacLibrary)
	f := &fakeDelete{answer: `{"source":"mac-library","deleted":["F/IMG_0001.HEIC","F/IMG_0002.HEIC"],"failed":[]}`}
	f.install(t)
	out := runWithInput(t, "y\n", "clean", "photos", "--apply")
	if len(f.calls) != 1 || f.calls[0][2] != iphone.SourceMacLibrary || !strings.Contains(out, "Recently Deleted for 30 days") {
		t.Errorf("calls %q, output %q", f.calls, out)
	}
	if ids := remaining(t, home); len(ids) != 0 {
		t.Errorf("review still holds %q", ids)
	}
}

func TestCleanPhotosHelperErrorKeepsReview(t *testing.T) {
	home := fakeHome(t)
	saveSelection(t, home, iphone.SourceDevice)
	f := &fakeDelete{answer: `{"error":"locked"}`}
	f.install(t)
	root := NewRoot()
	root.SetArgs([]string{"clean", "photos", "--apply"})
	root.SetIn(strings.NewReader("2\n"))
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "nothing was deleted: the iPhone is locked") {
		t.Errorf("err = %v", err)
	}
	if ids := remaining(t, home); len(ids) != 2 {
		t.Errorf("review now holds %q, want both photos", ids)
	}
}
