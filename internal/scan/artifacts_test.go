package scan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func gitRepo(t *testing.T, dir string, age time.Duration) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	writeFile(t, filepath.Join(dir, "README"), 10)
	date := time.Now().Add(-age).Format(time.RFC3339)
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "README"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "x"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func setAge(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func scanArtifacts(t *testing.T, roots []string, minBytes int64, exclude ...string) Findings {
	t.Helper()
	a := Artifacts{Size: size.New(time.Minute, 2), Roots: roots, Exclude: exclude, StaleDays: 90, MinBytes: minBytes}
	f, err := a.Scan(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const day = 24 * time.Hour

func TestArtifactsStaleGitProjectIsSafe(t *testing.T) {
	root := t.TempDir()
	gitRepo(t, filepath.Join(root, "old"), 200*day)
	writeFile(t, filepath.Join(root, "old/node_modules/x/index.js"), 50_000)
	gitRepo(t, filepath.Join(root, "new"), day)
	writeFile(t, filepath.Join(root, "new/node_modules/x/index.js"), 50_000)

	items := byLabel(scanArtifacts(t, []string{root}, 1).Items)
	base := filepath.Base(root)

	old := items[base+"/old/node_modules"]
	if old.Safety != Safe || old.Note != "last commit 200 days ago" {
		t.Errorf("old = %+v", old)
	}
	if want := "rm -rf " + ShellQuote(filepath.Join(root, "old/node_modules")); old.Hint != want {
		t.Errorf("hint = %q, want %q", old.Hint, want)
	}
	if n := items[base+"/new/node_modules"]; n.Safety != Review || n.Note != "last commit 1 days ago" {
		t.Errorf("new = %+v", n)
	}
}

func TestArtifactsNonGitUsesNewestOwnFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "app/main.py")
	writeFile(t, src, 10)
	setAge(t, src, 300*day)
	writeFile(t, filepath.Join(root, "app/node_modules/fresh.js"), 50_000)

	it := byLabel(scanArtifacts(t, []string{root}, 1).Items)[filepath.Base(root)+"/app/node_modules"]
	if it.Note != "last file change 300 days ago" || it.Safety != Safe {
		t.Fatalf("got %+v; artifact files must not count as activity", it)
	}
}

func TestArtifactsDetectionRules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "rs/Cargo.toml"), 10)
	writeFile(t, filepath.Join(root, "rs/target/debug/bin"), 10_000)
	writeFile(t, filepath.Join(root, "notrust/target/x"), 10_000)
	writeFile(t, filepath.Join(root, "web/.next/cache/x"), 10_000)
	writeFile(t, filepath.Join(root, "net/app.csproj"), 10)
	writeFile(t, filepath.Join(root, "net/bin/app.dll"), 10_000)
	writeFile(t, filepath.Join(root, "net/obj/x"), 10_000)
	writeFile(t, filepath.Join(root, "notnet/bin/tool"), 10_000)
	writeFile(t, filepath.Join(root, "py/.venv/pyvenv.cfg"), 0)
	writeFile(t, filepath.Join(root, "py/.venv/lib/x"), 10_000)
	writeFile(t, filepath.Join(root, "py/venv/lib/x"), 10_000)
	writeFile(t, filepath.Join(root, "js/node_modules/a/node_modules/b/x"), 10_000)

	items := byLabel(scanArtifacts(t, []string{root}, 1).Items)
	base := filepath.Base(root)
	want := []string{"rs/target", "web/.next", "net/bin", "net/obj", "py/.venv", "js/node_modules"}
	for _, w := range want {
		if _, ok := items[base+"/"+w]; !ok {
			t.Errorf("missing %s", w)
		}
	}
	if len(items) != len(want) {
		t.Errorf("got %d items, want %d: %v", len(items), len(want), keys(items))
	}
	rs := items[base+"/rs/target"]
	if wantHint := "cargo clean --manifest-path " + ShellQuote(filepath.Join(root, "rs/Cargo.toml")); rs.Hint != wantHint {
		t.Errorf("target hint = %q", rs.Hint)
	}
}

func TestArtifactsPycacheTotalledPerProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "py/__pycache__/a.pyc"), 10_000)
	writeFile(t, filepath.Join(root, "py/pkg/__pycache__/b.pyc"), 10_000)

	f := scanArtifacts(t, []string{root}, 1)
	if len(f.Items) != 1 {
		t.Fatalf("got %+v, want one row", f.Items)
	}
	it := f.Items[0]
	if it.Note != "2 folders" || it.Safety != Safe || it.Path != filepath.Join(root, "py") {
		t.Errorf("got %+v", it)
	}
	if !strings.HasPrefix(it.Hint, "find "+ShellQuote(filepath.Join(root, "py"))+" -name __pycache__") {
		t.Errorf("hint = %q", it.Hint)
	}
}

func TestArtifactsBrokenVenvShownEvenWhenSmall(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "py/.venv/pyvenv.cfg")
	writeFile(t, cfg, 0)
	missing := filepath.Join(root, "gone/pyenv/3.11/bin")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("home = %s\nversion = 3.11\n", missing)), 0o600); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(root, "fine/.venv/pyvenv.cfg")
	writeFile(t, ok, 0)
	if err := os.WriteFile(ok, []byte("home = "+root+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := scanArtifacts(t, []string{root}, 1<<30)
	if len(f.Items) != 1 {
		t.Fatalf("got %+v, want only the broken venv", f.Items)
	}
	if it := f.Items[0]; it.Safety != Safe || !strings.Contains(it.Note, "broken") || !strings.Contains(it.Note, missing) {
		t.Errorf("got %+v", it)
	}
}

func TestArtifactsExcludeMinSizeAndMissingRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skip/node_modules/x"), 50_000)
	writeFile(t, filepath.Join(root, "tiny/node_modules/x"), 10)

	f := scanArtifacts(t, []string{root, filepath.Join(root, "missing")}, 20_000, filepath.Join(root, "skip"))
	if len(f.Items) != 0 || len(f.Skipped) != 0 {
		t.Fatalf("got %+v", f)
	}
}

func TestArtifactsSkipsGitFolder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "p/.git/node_modules/x"), 10_000)

	if f := scanArtifacts(t, []string{root}, 1); len(f.Items) != 0 {
		t.Fatalf("found artifacts inside .git: %+v", f.Items)
	}
}

func TestArtifactsRecordsUnreadableFolder(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	writeFile(t, filepath.Join(locked, "node_modules/x"), 10)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	f := scanArtifacts(t, []string{root}, 1)
	if len(f.Skipped) != 1 || f.Skipped[0].Path != locked || f.Skipped[0].Reason != "permission" {
		t.Fatalf("skipped = %+v", f.Skipped)
	}
}

func keys(m map[string]Item) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}
