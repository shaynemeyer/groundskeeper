package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// twoRuns writes a scan run and then a clean run to a log file.
func twoRuns(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gk.log")
	scan, c1, err := Open(path, Options{Level: "debug", Cmd: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	scan.Debug("detail")
	scan.Info("scanner done", ID("scanner", "Caches"), Count("items", 3))
	scan.Warn("path skipped", Path("/h/Library/Mail"), ID("reason", "permission"))
	c1.Close()
	clean, c2, err := Open(path, Options{Level: "debug", Cmd: "clean browsers"})
	if err != nil {
		t.Fatal(err)
	}
	clean.Info("clean browsers", Path("/h/cache"), ID("outcome", "would move"))
	clean.Error("failed", ErrKind(os.ErrPermission))
	c2.Close()
	return path
}

func msgs(entries []Entry) []string {
	var m []string
	for _, e := range entries {
		m = append(m, e.Msg)
	}
	return m
}

func TestReadFilters(t *testing.T) {
	path := twoRuns(t)
	cases := []struct {
		name string
		f    Filter
		want []string
	}{
		{"all", Filter{MinLevel: slog.LevelDebug}, []string{"detail", "scanner done", "path skipped", "clean browsers", "failed"}},
		{"warn and up", Filter{MinLevel: slog.LevelWarn}, []string{"path skipped", "failed"}},
		{"last run", Filter{MinLevel: slog.LevelDebug, Run: "last"}, []string{"clean browsers", "failed"}},
		{"command prefix", Filter{MinLevel: slog.LevelDebug, Cmd: "clean"}, []string{"clean browsers", "failed"}},
		{"limit keeps newest", Filter{MinLevel: slog.LevelDebug, Limit: 2}, []string{"clean browsers", "failed"}},
	}
	for _, c := range cases {
		got, _, err := Read(path, c.f)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(msgs(got), c.want) {
			t.Errorf("%s: got %v, want %v", c.name, msgs(got), c.want)
		}
	}
}

func TestReadParsesFields(t *testing.T) {
	got, _, err := Read(twoRuns(t), Filter{Cmd: "scan", MinLevel: slog.LevelWarn})
	if err != nil {
		t.Fatal(err)
	}
	e := got[0]
	if e.Level != slog.LevelWarn || e.Cmd != "scan" || len(e.RunID) != 12 || e.Time.IsZero() {
		t.Fatalf("entry = %+v", e)
	}
	if !slices.Equal(e.Attrs, []string{"path=/h/Library/Mail", "reason=permission"}) {
		t.Fatalf("attrs = %v", e.Attrs)
	}
}

func TestFollowSeesNewLinesAndPartialLines(t *testing.T) {
	path := twoRuns(t)
	_, offset, err := Read(path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"time":"2026-10-25T17:00:00Z","level":"INFO","msg":"new","run_id":"r","cmd":"scan"}` + "\n")
	f.WriteString(`{"time":"2026-10-25T17:00:01Z","level":"INFO","msg":"half`)
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	var got []string
	Follow(ctx, path, offset, Filter{}, func(e Entry) { got = append(got, e.Msg) })
	if !slices.Equal(got, []string{"new"}) {
		t.Fatalf("followed %v, want only the complete new line", got)
	}
}

func TestReadKeepsLargeNumbersReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gk.log")
	l, c, err := Open(path, Options{Level: "info", Cmd: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("run finished", Bytes(43_041_574_912))
	c.Close()

	got, _, err := Read(path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got[0].Attrs, []string{"bytes=43041574912"}) {
		t.Fatalf("attrs = %v", got[0].Attrs)
	}
}
