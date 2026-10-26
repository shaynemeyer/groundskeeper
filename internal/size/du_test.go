package size

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newMeasurer() *Measurer { return New(time.Minute, 2) }

func TestMeasureCountsAllocatedBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), 10_000)

	got := newMeasurer().Measure(context.Background(), dir).Bytes
	if got < 10_000 {
		t.Fatalf("got %d bytes, want at least 10000", got)
	}
}

func TestMeasureSparseFileUsesBlocks(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got := newMeasurer().Measure(context.Background(), dir).Bytes
	if got >= 1<<20 {
		t.Fatalf("sparse 1 GB file measured %d bytes, want under 1 MB", got)
	}
}

func TestMeasureCountsHardlinksOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), 100_000)
	m := newMeasurer()
	single := New(time.Minute, 1).Measure(context.Background(), dir).Bytes

	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if got := m.Measure(context.Background(), dir).Bytes; got != single {
		t.Fatalf("with hardlink got %d, want %d", got, single)
	}
}

func TestMeasureSkipsSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "big"), 1_000_000)
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "big"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	if got := newMeasurer().Measure(context.Background(), dir).Bytes; got >= 1_000_000 {
		t.Fatalf("symlink target was counted: %d bytes", got)
	}
}

func TestMeasureRecordsPermissionSkip(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	writeFile(t, filepath.Join(locked, "f"), 10)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	res := newMeasurer().Measure(context.Background(), dir)
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != "permission" || res.Skipped[0].Path != locked {
		t.Fatalf("skipped = %+v, want one permission skip for %s", res.Skipped, locked)
	}
}

func TestMeasureRecordsTimeout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := newMeasurer().Measure(ctx, dir)
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != "timeout" {
		t.Fatalf("skipped = %+v, want one timeout", res.Skipped)
	}
}

func TestMeasureMissingRootIsZero(t *testing.T) {
	res := newMeasurer().Measure(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if res.Bytes != 0 || len(res.Skipped) != 0 {
		t.Fatalf("got %+v, want zero result", res)
	}
}

func TestMeasureAllKeepsOrder(t *testing.T) {
	small, big := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(small, "f"), 10)
	writeFile(t, filepath.Join(big, "f"), 1_000_000)

	res := newMeasurer().MeasureAll(context.Background(), []string{small, big})
	if res[0].Path != small || res[1].Path != big || res[0].Bytes >= res[1].Bytes {
		t.Fatalf("got %+v", res)
	}
}

func TestStatsCountsWork(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub/a"), 10)
	if err := os.Link(filepath.Join(dir, "sub/a"), filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	m := newMeasurer()
	m.Measure(context.Background(), dir)

	if got := m.Stats(); got != (Stats{Files: 2, Dirs: 2, HardlinksDeduped: 1}) {
		t.Fatalf("stats = %+v", got)
	}
}
