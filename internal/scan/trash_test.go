package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func TestTrashCachesAndDownloads(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".npm/f"), 10_000)
	writeFile(t, filepath.Join(home, "Library/Caches/com.example.app/f"), 10_000)
	old := filepath.Join(home, "Downloads/old.dmg")
	writeFile(t, old, 10_000)
	setAge(t, old, 200*day)

	caches := byLabel(scanCaches(t, home, 1).Items)
	if got := caches["npm"].Trash; got != filepath.Join(home, ".npm") {
		t.Errorf("npm Trash = %q", got)
	}
	if got := caches["com.example.app"].Trash; got != filepath.Join(home, "Library/Caches/com.example.app") {
		t.Errorf("Library cache Trash = %q", got)
	}
	if got := byLabel(scanDownloads(t, home).Items)["old.dmg"].Trash; got != old {
		t.Errorf("download Trash = %q", got)
	}
}

func TestTrashArtifactsButNotPycacheTotals(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, "web/node_modules")
	writeFile(t, filepath.Join(modules, "x"), 10_000)
	writeFile(t, filepath.Join(root, "py/a/__pycache__/x.pyc"), 10_000)
	writeFile(t, filepath.Join(root, "py/b/__pycache__/x.pyc"), 10_000)

	for _, it := range scanArtifacts(t, []string{root}, 1).Items {
		want := ""
		if strings.HasSuffix(it.Label, "node_modules") {
			want = modules
		}
		if it.Trash != want {
			t.Errorf("%s Trash = %q, want %q", it.Label, it.Trash, want)
		}
	}
}

func TestTrashModels(t *testing.T) {
	home := ollamaHome(t)
	hub := filepath.Join(home, ".cache/huggingface/hub")
	writeFile(t, filepath.Join(hub, "models--org--plain/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "datasets--glue/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "spaces--org--demo/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "blobs/ab/xethash"), 10_000)
	linked := filepath.Join(hub, "models--org--linked/blobs/etag")
	if err := os.MkdirAll(filepath.Dir(linked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../blobs/ab/xethash", linked); err != nil {
		t.Fatal(err)
	}
	lm := filepath.Join(home, ".lmstudio/models/pub/model")
	writeFile(t, filepath.Join(lm, "q4.gguf"), 10_000)
	writeFile(t, filepath.Join(home, "ComfyUI/models/loras/x.safetensors"), 10_000)

	want := map[string]string{
		"Hugging Face: model/org/plain": filepath.Join(hub, "models--org--plain"),
		"Hugging Face: dataset/glue":    filepath.Join(hub, "datasets--glue"),
		"LM Studio: pub/model":          lm,
	}
	items := scanModels(t, home).Items
	if len(items) < 8 {
		t.Fatalf("got %v", keys(byLabel(items)))
	}
	for _, it := range items {
		if it.Trash != want[it.Label] {
			t.Errorf("%s Trash = %q, want %q", it.Label, it.Trash, want[it.Label])
		}
	}
}

func TestTrashNeverSetForToolchainsOrBrowsers(t *testing.T) {
	home, _ := chromeHome(t)
	writeFile(t, filepath.Join(home, ".rustup/toolchains/stable-aarch64-apple-darwin/bin/rustc"), 10_000)

	tools, err := Toolchains{Size: size.New(time.Minute, 2), MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	items := append(tools.Items, scanBrowsers(t, home).Items...)
	if len(items) < 2 {
		t.Fatalf("got %v", keys(byLabel(items)))
	}
	for _, it := range items {
		if it.Trash != "" {
			t.Errorf("%s Trash = %q, want none", it.Label, it.Trash)
		}
	}
}
