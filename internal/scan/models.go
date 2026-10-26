package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

// hfRepoTypes maps cache folder prefixes to the type hf cache rm expects.
var hfRepoTypes = map[string]string{"models": "model", "datasets": "dataset", "spaces": "space"}

// Models finds Ollama models, Hugging Face cache repos, LM Studio models and
// ComfyUI model folders. Locations are the tools' defaults, since a
// scheduled run doesn't see shell variables such as OLLAMA_MODELS.
type Models struct {
	Size         *size.Measurer
	ProjectRoots []string // absolute; searched for ComfyUI installs
	MinBytes     int64
}

// Name returns the scanner name.
func (m Models) Name() string { return "AI models" }

// Scan collects every model location and measures the ones without known sizes.
func (m Models) Scan(ctx context.Context, home string) (Findings, error) {
	f := ollamaModels(filepath.Join(home, ".ollama", "models"))

	hub := filepath.Join(home, ".cache", "huggingface", "hub")
	hf := hfRepos(hub)
	measured := append(hf, lmStudioModels(home)...)
	measured = append(measured, comfyModels(home, m.ProjectRoots)...)
	f.Skipped = append(f.Skipped, measure(ctx, m.Size, measured)...)
	for i := range hf {
		measured[i].Bytes += hfSharedBytes(hub, measured[i].Path)
	}

	f.Items = keep(append(f.Items, measured...), m.MinBytes)
	return f, nil
}

// hfRepos lists repos in the Hugging Face hub cache, such as
// models--org--name, as model/org/name.
func hfRepos(hub string) []Item {
	var items []Item
	for _, e := range readDirs(hub) {
		prefix, rest, ok := strings.Cut(e, "--")
		kind := hfRepoTypes[prefix]
		if !ok || kind == "" {
			continue
		}
		id := kind + "/" + strings.ReplaceAll(rest, "--", "/")
		items = append(items, Item{
			Section: "AI models",
			Label:   "Hugging Face: " + id,
			Path:    filepath.Join(hub, e),
			Safety:  Review,
			Hint:    "hf cache rm " + ShellQuote(id),
		})
	}
	return items
}

// hfSharedBytes adds the files a repo links into the hub's shared blob
// store, which the size engine skips because they are symlinks.
func hfSharedBytes(hub, repo string) int64 {
	store, err := filepath.EvalSymlinks(filepath.Join(hub, "blobs"))
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(filepath.Join(repo, "blobs"))
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := filepath.EvalSymlinks(filepath.Join(repo, "blobs", e.Name()))
		if err != nil || !strings.HasPrefix(target, store+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(target); err == nil {
			total += size.Allocated(info)
		}
	}
	return total
}

// lmStudioModels lists models/<publisher>/<model> in LM Studio's current
// and older locations.
func lmStudioModels(home string) []Item {
	var items []Item
	for _, rel := range []string{".lmstudio/models", ".cache/lm-studio/models"} {
		root := filepath.Join(home, rel)
		for _, pub := range readDirs(root) {
			for _, model := range readDirs(filepath.Join(root, pub)) {
				items = append(items, Item{
					Section: "AI models",
					Label:   "LM Studio: " + pub + "/" + model,
					Path:    filepath.Join(root, pub, model),
					Safety:  Review,
					Note:    "delete in LM Studio under My Models",
				})
			}
		}
	}
	return items
}

// comfyModels lists the folders in ComfyUI's models directory for installs
// in ~, ~/Documents, and at most two levels into each project root.
func comfyModels(home string, roots []string) []Item {
	installs := []string{filepath.Join(home, "ComfyUI"), filepath.Join(home, "Documents", "ComfyUI")}
	for _, root := range roots {
		installs = append(installs, filepath.Join(root, "ComfyUI"))
		for _, project := range readDirs(root) {
			installs = append(installs, filepath.Join(root, project, "ComfyUI"))
		}
	}
	var items []Item
	for _, install := range installs {
		models := filepath.Join(install, "models")
		for _, kind := range readDirs(models) {
			items = append(items, Item{
				Section: "AI models",
				Label:   "ComfyUI: " + kind,
				Path:    filepath.Join(models, kind),
				Safety:  Review,
			})
		}
	}
	return items
}

// readDirs returns the names of real folders in dir, skipping hidden ones
// and symlinks. A missing or unreadable dir has none.
func readDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names
}
