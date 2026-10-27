package clean

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
)

// realHome is a fake home without symlinks in its path; t.TempDir is
// under /var, which is a symlink to /private/var on macOS.
func realHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func mkfile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	mkfile(t, filepath.Join(path, "f"))
	return path
}

func symlink(t *testing.T, target, link string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func macRules(home string) MacRules {
	return MacRules{Home: home, Roots: []string{filepath.Join(home, "projects")}}
}

func TestMacRulesAllowEachSection(t *testing.T) {
	home := realHome(t)
	projects := filepath.Join(home, "projects")
	mkfile(t, filepath.Join(projects, "rust/Cargo.toml"))
	mkfile(t, filepath.Join(projects, "py/.venv/pyvenv.cfg"))
	mkdir(t, filepath.Join(home, "Library/Caches/Google/Chrome"))
	cases := []MacItem{
		{Section: "Caches", Trash: mkdir(t, filepath.Join(home, ".npm")), Dir: true},
		{Section: "Caches", Trash: mkdir(t, filepath.Join(home, "Library/Caches/com.example.app")), Dir: true},
		{Section: "Caches", Trash: mkdir(t, filepath.Join(home, "Library/Caches/Google/Other")), Dir: true},
		{Section: "Build artifacts", Trash: mkdir(t, filepath.Join(projects, "web/node_modules")), Dir: true},
		{Section: "Build artifacts", Trash: mkdir(t, filepath.Join(projects, "rust/target")), Dir: true},
		{Section: "Build artifacts", Trash: filepath.Join(projects, "py/.venv"), Dir: true},
		{Section: "Downloads", Trash: mkfile(t, filepath.Join(home, "Downloads/old.dmg"))},
		{Section: "Downloads", Trash: mkdir(t, filepath.Join(home, "Downloads/old-folder")), Dir: true},
		{Section: "AI models", Trash: mkdir(t, filepath.Join(home, ".cache/huggingface/hub/models--org--name")), Dir: true},
		{Section: "AI models", Trash: mkdir(t, filepath.Join(home, ".cache/huggingface/hub/datasets--glue")), Dir: true},
		{Section: "AI models", Trash: mkdir(t, filepath.Join(home, ".lmstudio/models/pub/model")), Dir: true},
		{Section: "AI models", Trash: mkdir(t, filepath.Join(home, ".cache/lm-studio/models/pub/model")), Dir: true},
	}
	for _, it := range cases {
		if reason := macRules(home).Check(it); reason != "" {
			t.Errorf("%s %s refused: %s", it.Section, it.Trash, reason)
		}
	}
}

func TestMacRulesRefuse(t *testing.T) {
	home := realHome(t)
	projects := filepath.Join(home, "projects")
	documents := mkdir(t, filepath.Join(home, "Documents/taxes"))
	outside := mkdir(t, filepath.Join(realHome(t), "Downloads/x"))
	hub := filepath.Join(home, ".cache/huggingface/hub")
	mkfile(t, filepath.Join(hub, "blobs/ab/xet"))
	symlink(t, "../../blobs/ab/xet", filepath.Join(hub, "models--org--linked/blobs/etag"))
	mkdir(t, filepath.Join(home, "elsewhere/node_modules"))
	symlink(t, filepath.Join(home, "elsewhere"), filepath.Join(home, "Downloads/via"))

	cases := []struct {
		it     MacItem
		reason string
	}{
		{MacItem{Section: "Downloads", Trash: documents, Dir: true}, "not something Downloads can clean"},
		{MacItem{Section: "Caches", Trash: documents, Dir: true}, "not something Caches can clean"},
		{MacItem{Section: "Downloads", Trash: outside, Dir: true}, "outside the home folder"},
		{MacItem{Section: "Downloads", Trash: home + "/Downloads/../Documents/taxes", Dir: true}, "path is not clean"},
		{MacItem{Section: "Downloads", Trash: "Downloads/x", Dir: true}, "path is not clean"},
		{MacItem{Section: "Downloads", Trash: symlink(t, documents, filepath.Join(home, "Downloads/link")), Dir: true}, "path is a symlink"},
		{MacItem{Section: "Downloads", Trash: filepath.Join(home, "Downloads/via/node_modules"), Dir: true}, "path goes through a symlink"},
		{MacItem{Section: "Downloads", Trash: mkfile(t, filepath.Join(home, "Downloads/was-a-folder")), Dir: true}, "changed from a folder to a file, or back"},
		{MacItem{Section: "Downloads", Trash: filepath.Join(home, "Downloads/gone"), Dir: true}, "no longer there"},
		{MacItem{Section: "Downloads", Trash: mkfile(t, filepath.Join(home, "Downloads/.hidden"))}, "not something Downloads can clean"},
		{MacItem{Section: "Caches", Trash: mkdir(t, filepath.Join(home, "Library/Caches/Google/Chrome")), Dir: true}, "not something Caches can clean"},
		{MacItem{Section: "Caches", Trash: filepath.Join(home, "Library/Caches/Google"), Dir: true}, "not something Caches can clean"},
		{MacItem{Section: "Build artifacts", Trash: mkdir(t, filepath.Join(projects, "py/__pycache__")), Dir: true}, "not something Build artifacts can clean"},
		{MacItem{Section: "Build artifacts", Trash: filepath.Join(home, "elsewhere/node_modules"), Dir: true}, "not something Build artifacts can clean"},
		{MacItem{Section: "Build artifacts", Trash: mkdir(t, filepath.Join(projects, "web/src")), Dir: true}, "not something Build artifacts can clean"},
		{MacItem{Section: "AI models", Trash: filepath.Join(hub, "models--org--linked"), Dir: true}, "not something AI models can clean"},
		{MacItem{Section: "AI models", Trash: mkdir(t, filepath.Join(hub, "spaces--org--demo")), Dir: true}, "not something AI models can clean"},
		{MacItem{Section: "AI models", Trash: mkdir(t, filepath.Join(home, ".ollama/models/blobs")), Dir: true}, "not something AI models can clean"},
		{MacItem{Section: "Toolchains", Trash: mkdir(t, filepath.Join(home, ".rustup/toolchains/stable")), Dir: true}, "not something Toolchains can clean"},
	}
	for _, c := range cases {
		if got := macRules(home).Check(c.it); got != c.reason {
			t.Errorf("%s %s: got %q, want %q", c.it.Section, c.it.Trash, got, c.reason)
		}
	}
}

func TestNewMacReview(t *testing.T) {
	home := realHome(t)
	folder := mkdir(t, filepath.Join(home, ".npm"))
	file := mkfile(t, filepath.Join(home, "Downloads/old.dmg"))
	items := []history.Item{
		{Section: "Caches", Label: "npm", Path: folder, Bytes: 300, Trash: folder},
		{Section: "Downloads", Label: "old.dmg", Path: file, Bytes: 200, Trash: file},
	}

	r, err := NewMacReview(reviewed, items)
	if err != nil {
		t.Fatal(err)
	}
	want := []MacItem{
		{Section: "Caches", Label: "npm", Trash: folder, Bytes: 300, Dir: true},
		{Section: "Downloads", Label: "old.dmg", Trash: file, Bytes: 200},
	}
	if len(r.Items) != 2 || r.Items[0] != want[0] || r.Items[1] != want[1] || !r.Time.Equal(reviewed) {
		t.Errorf("got %+v", r)
	}
	if r.Bytes() != 500 {
		t.Errorf("bytes %d, want 500", r.Bytes())
	}
	if left := r.Without([]string{folder}); len(left.Items) != 1 || left.Items[0].Trash != file || len(r.Items) != 2 {
		t.Errorf("Without = %+v, original %+v", left, r)
	}
}

func TestNewMacReviewRefuses(t *testing.T) {
	home := realHome(t)
	for _, it := range []history.Item{
		{Section: "Build artifacts", Label: "py/__pycache__", Path: home},
		{Section: "Downloads", Label: "gone", Trash: filepath.Join(home, "Downloads/gone")},
	} {
		if _, err := NewMacReview(reviewed, []history.Item{it}); err == nil {
			t.Errorf("%s: want refusal", it.Label)
		}
	}
}

func TestMacReviewRoundTrip(t *testing.T) {
	home := realHome(t)
	path := MacReviewPath(home)
	if _, err := LoadMacReview(path); !errors.Is(err, ErrNoMacReview) {
		t.Fatalf("before saving: %v", err)
	}
	r := MacReview{Time: reviewed, Items: []MacItem{{Section: "Caches", Label: "npm", Trash: filepath.Join(home, ".npm"), Bytes: 300, Dir: true}}}
	if err := SaveMacReview(path, r); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMacReview(path)
	if err != nil || len(got.Items) != 1 || got.Items[0] != r.Items[0] || !got.Time.Equal(reviewed) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("review file: %v %v", info, err)
	}
}

func TestMacCleanMovesOnlyAllowedItems(t *testing.T) {
	home := realHome(t)
	npm := mkdir(t, filepath.Join(home, ".npm"))
	stuck := mkfile(t, filepath.Join(home, "Downloads/stuck.dmg"))
	documents := mkdir(t, filepath.Join(home, "Documents/taxes"))
	var trashed []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != trashTool || len(args) != 2 || args[0] != "-s" {
			t.Fatalf("ran %s %v", name, args)
		}
		if args[1] == stuck {
			return nil, errors.New("exit status 1")
		}
		trashed = append(trashed, args[1])
		return nil, nil
	}
	r := MacReview{Items: []MacItem{
		{Section: "Caches", Label: "npm", Trash: npm, Bytes: 300, Dir: true},
		{Section: "Downloads", Label: "taxes", Trash: documents, Bytes: 200, Dir: true},
		{Section: "Downloads", Label: "stuck.dmg", Trash: stuck, Bytes: 100},
	}}

	got := Mac{Rules: macRules(home), Run: run, Log: logging.Discard()}.Clean(context.Background(), r)
	want := []string{Moved, Refused, Failed}
	for i, a := range got {
		if a.Outcome != want[i] {
			t.Errorf("%s: outcome %q (%s), want %q", a.Item.Label, a.Outcome, a.Reason, want[i])
		}
	}
	if len(trashed) != 1 || trashed[0] != npm {
		t.Errorf("trashed %v, want only npm", trashed)
	}
}
