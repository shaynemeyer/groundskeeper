package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T, opts Options) (string, *Logger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "groundskeeper.log")
	l, c, err := Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return path, l
}

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	for line := range strings.Lines(string(data)) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestOpenWritesJSONWithRunIDAndCmd(t *testing.T) {
	path, l := openTest(t, Options{Level: "info", Cmd: "scan"})
	l.Info("scanner done", ID("scanner", "Caches"), Bytes(42), Duration(1500*time.Millisecond))
	l.Debug("hidden at info level")

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	got := lines[0]
	if got["cmd"] != "scan" || got["scanner"] != "Caches" || got["bytes"] != 42.0 || got["duration_ms"] != 1500.0 {
		t.Fatalf("line = %v", got)
	}
	if id, _ := got["run_id"].(string); len(id) != 12 {
		t.Fatalf("run_id = %v", got["run_id"])
	}
}

func TestOpenFilePermissions(t *testing.T) {
	path, l := openTest(t, Options{Level: "info"})
	l.Info("x")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v, want 0600", info.Mode().Perm())
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v, want 0700", dir.Mode().Perm())
	}
}

func TestVerboseWriterGetsDebug(t *testing.T) {
	var buf bytes.Buffer
	_, l := openTest(t, Options{Level: "info", Verbose: &buf})
	l.Debug("detail", Count("items", 3))

	if !strings.Contains(buf.String(), "detail") || !strings.Contains(buf.String(), "items=3") {
		t.Fatalf("verbose output = %q", buf.String())
	}
}

func TestOpenRejectsBadLevel(t *testing.T) {
	if _, _, err := Open(filepath.Join(t.TempDir(), "l.log"), Options{Level: "loud"}); err == nil {
		t.Fatal("want error for unknown level")
	}
}

func TestRotationKeepsThreeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groundskeeper.log")
	for i := range 5 {
		if err := os.WriteFile(path, []byte(fmt.Sprintf("run %d\n", i)+strings.Repeat("x", 100)), 0o600); err != nil {
			t.Fatal(err)
		}
		_, c, err := Open(path, Options{Level: "info", MaxBytes: 50})
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}

	for i, want := range map[int]string{1: "run 4", 2: "run 3", 3: "run 2"} {
		data, err := os.ReadFile(fmt.Sprintf("%s.%d", path, i))
		if err != nil || !strings.HasPrefix(string(data), want) {
			t.Errorf(".%d = %q, %v; want %q", i, data, err, want)
		}
	}
	if _, err := os.Stat(path + ".4"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".4 should not exist")
	}
}

func TestErrKindHidesMessage(t *testing.T) {
	secret := errors.New("token=abc123")
	cases := map[error]string{
		fs.ErrPermission:                       "permission",
		fmt.Errorf("open: %w", fs.ErrNotExist): "not_exist",
		context.DeadlineExceeded:               "timeout",
		secret:                                 "other",
	}
	for err, want := range cases {
		if got := ErrKind(err).attr.Value.String(); got != want {
			t.Errorf("ErrKind(%v) = %s, want %s", err, got, want)
		}
	}
}
