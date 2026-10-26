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

func writeText(t *testing.T, path, text string) {
	t.Helper()
	writeFile(t, path, 0)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanModels(t *testing.T, home string, roots ...string) Findings {
	t.Helper()
	f, err := Models{Size: size.New(time.Minute, 2), ProjectRoots: roots, MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const llamaManifest = `{"schemaVersion":2,
 "config":{"digest":"sha256:c0","size":500},
 "layers":[{"digest":"sha256:aa","size":2000000000},{"digest":"sha256:lic","size":1500}]}`

const hfManifest = `{"config":{"digest":"sha256:c1","size":500},
 "layers":[{"digest":"sha256:bb","size":4000000000},{"digest":"sha256:lic","size":1500}]}`

func ollamaHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	models := filepath.Join(home, ".ollama/models")
	llama := filepath.Join(models, "manifests/registry.ollama.ai/library/llama3.2/latest")
	writeText(t, llama, llamaManifest)
	setAge(t, llama, 40*day)
	writeText(t, filepath.Join(models, "manifests/hf.co/org/repo/Q4_K_M"), hfManifest)
	for _, b := range []string{"sha256-c0", "sha256-aa", "sha256-lic", "sha256-c1", "sha256-bb"} {
		writeFile(t, filepath.Join(models, "blobs", b), 10)
	}
	writeFile(t, filepath.Join(models, "blobs/sha256-old"), 100_000)
	writeFile(t, filepath.Join(models, "blobs/sha256-new-partial"), 100_000)
	return home
}

func TestOllamaModelsFromManifests(t *testing.T) {
	items := byLabel(scanModels(t, ollamaHome(t)).Items)

	llama := items["Ollama: llama3.2:latest"]
	if llama.Bytes != 2_000_002_000 || llama.Hint != "ollama rm 'llama3.2:latest'" || llama.Note != "pulled 40 days ago" {
		t.Errorf("llama = %+v", llama)
	}
	if hf := items["Ollama: hf.co/org/repo:Q4_K_M"]; hf.Bytes != 4_000_002_000 {
		t.Errorf("hf model = %+v", hf)
	}
	orphans := items["Ollama: blobs no model uses"]
	if orphans.Note != "2 files not used by any model" || orphans.Bytes < 200_000 || orphans.Safety != Review {
		t.Errorf("orphans = %+v", orphans)
	}
}

func TestOllamaSkipsOrphansWhenManifestsV2HasFiles(t *testing.T) {
	home := ollamaHome(t)
	writeText(t, filepath.Join(home, ".ollama/models/manifests-v2/x/y"), "{}")

	if _, ok := byLabel(scanModels(t, home).Items)["Ollama: blobs no model uses"]; ok {
		t.Fatal("orphans reported although manifests-v2 has files")
	}
}

func TestOllamaBadManifestIsSkipped(t *testing.T) {
	home := t.TempDir()
	bad := filepath.Join(home, ".ollama/models/manifests/registry.ollama.ai/library/x/latest")
	writeText(t, bad, "{not json")

	f := scanModels(t, home)
	if len(f.Skipped) != 1 || f.Skipped[0].Path != bad || f.Skipped[0].Reason != "error" {
		t.Fatalf("skipped = %+v", f.Skipped)
	}
}

func TestOllamaName(t *testing.T) {
	cases := map[string]string{
		"registry.ollama.ai/library/llama3.2/latest": "llama3.2:latest",
		"registry.ollama.ai/jmorgan/tiny/1b":         "jmorgan/tiny:1b",
		"hf.co/bartowski/model-GGUF/Q4_K_M":          "hf.co/bartowski/model-GGUF:Q4_K_M",
		"odd/path":                                   "odd/path",
	}
	for in, want := range cases {
		if got := ollamaName(in); got != want {
			t.Errorf("ollamaName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHuggingFaceRepos(t *testing.T) {
	home := t.TempDir()
	hub := filepath.Join(home, ".cache/huggingface/hub")
	writeFile(t, filepath.Join(hub, "models--julien-c--EsperBERTo-small/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "models--bert-base-cased/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "datasets--glue/blobs/abc"), 10_000)
	writeFile(t, filepath.Join(hub, "kernels--x--y/blobs/abc"), 10_000)

	items := byLabel(scanModels(t, home).Items)
	want := map[string]string{
		"Hugging Face: model/julien-c/EsperBERTo-small": "hf cache rm 'model/julien-c/EsperBERTo-small'",
		"Hugging Face: model/bert-base-cased":           "hf cache rm 'model/bert-base-cased'",
		"Hugging Face: dataset/glue":                    "hf cache rm 'dataset/glue'",
	}
	if len(items) != len(want) {
		t.Fatalf("got %v", keys(items))
	}
	for label, hint := range want {
		if items[label].Hint != hint {
			t.Errorf("%s hint = %q, want %q", label, items[label].Hint, hint)
		}
	}
}

func TestHuggingFaceCountsSharedBlobStore(t *testing.T) {
	home := t.TempDir()
	hub := filepath.Join(home, ".cache/huggingface/hub")
	shared := filepath.Join(hub, "blobs/ab/xethash")
	writeFile(t, shared, 1_000_000)
	repoBlob := filepath.Join(hub, "models--org--name/blobs/etag")
	if err := os.MkdirAll(filepath.Dir(repoBlob), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../blobs/ab/xethash", repoBlob); err != nil {
		t.Fatal(err)
	}

	it := byLabel(scanModels(t, home).Items)["Hugging Face: model/org/name"]
	if it.Bytes < 1_000_000 {
		t.Fatalf("repo using the shared store measured %d bytes", it.Bytes)
	}
}

func TestLMStudioAndComfyUI(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".lmstudio/models/lmstudio-community/Qwen-GGUF/q4.gguf"), 10_000)
	root := filepath.Join(home, "projects")
	writeFile(t, filepath.Join(root, "local-ai/ComfyUI/models/checkpoints/sdxl.safetensors"), 10_000)
	writeFile(t, filepath.Join(home, "ComfyUI/models/loras/x.safetensors"), 10_000)

	items := byLabel(scanModels(t, home, root).Items)
	for _, label := range []string{"LM Studio: lmstudio-community/Qwen-GGUF", "ComfyUI: checkpoints", "ComfyUI: loras"} {
		if it, ok := items[label]; !ok || it.Safety != Review || it.Hint != "" {
			t.Errorf("%s = %+v", label, it)
		}
	}
	if !strings.Contains(items["LM Studio: lmstudio-community/Qwen-GGUF"].Note, "My Models") {
		t.Error("LM Studio note missing")
	}
}

func TestModelsEmptyHome(t *testing.T) {
	if f := scanModels(t, t.TempDir()); len(f.Items) != 0 || len(f.Skipped) != 0 {
		t.Fatalf("got %+v", f)
	}
}
