package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

type ollamaLayer struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type ollamaManifest struct {
	Config ollamaLayer   `json:"config"`
	Layers []ollamaLayer `json:"layers"`
}

// ollamaModels reads manifests under dir (~/.ollama/models) and returns one
// item per model, plus one for blobs no model uses.
func ollamaModels(dir string) Findings {
	var f Findings
	refs := map[string]bool{}
	root := filepath.Join(dir, "manifests")
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				f.Skipped = append(f.Skipped, size.NewSkip(path, err))
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		it, digests, err := ollamaModel(root, path)
		if err != nil {
			f.Skipped = append(f.Skipped, size.NewSkip(path, err))
			return nil
		}
		for _, dg := range digests {
			refs[dg] = true
		}
		f.Items = append(f.Items, it)
		return nil
	})
	if orphans, ok := ollamaOrphans(dir, refs); ok {
		f.Items = append(f.Items, orphans)
	}
	return f
}

func ollamaModel(root, path string) (Item, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Item{}, nil, err
	}
	var m ollamaManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Item{}, nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Item{}, nil, err
	}
	rel, _ := filepath.Rel(root, path)
	name := ollamaName(rel)
	it := Item{
		Section: "AI models",
		Label:   "Ollama: " + name,
		Path:    path,
		Safety:  Review,
		Hint:    "ollama rm " + ShellQuote(name),
		Note:    fmt.Sprintf("pulled %d days ago", int(time.Since(info.ModTime()).Hours()/24)),
	}
	var digests []string
	for _, l := range append(m.Layers, m.Config) {
		it.Bytes += l.Size
		digests = append(digests, strings.Replace(l.Digest, ":", "-", 1))
	}
	return it, digests, nil
}

// ollamaName turns registry/namespace/model/tag into the name ollama shows.
func ollamaName(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 4 {
		return strings.Join(parts, "/")
	}
	host, ns, model, tag := parts[0], parts[1], parts[2], parts[3]
	switch {
	case host == "registry.ollama.ai" && ns == "library":
		return model + ":" + tag
	case host == "registry.ollama.ai":
		return ns + "/" + model + ":" + tag
	}
	return host + "/" + ns + "/" + model + ":" + tag
}

// ollamaOrphans totals blobs no manifest references. It is skipped when
// manifests-v2 holds files, since those manifests aren't parsed and every
// blob would look unused.
func ollamaOrphans(dir string, refs map[string]bool) (Item, bool) {
	if hasFiles(filepath.Join(dir, "manifests-v2")) {
		return Item{}, false
	}
	blobs := filepath.Join(dir, "blobs")
	entries, err := os.ReadDir(blobs)
	if err != nil {
		return Item{}, false
	}
	it := Item{Section: "AI models", Label: "Ollama: blobs no model uses", Path: blobs, Safety: Review}
	count := 0
	for _, e := range entries {
		if refs[e.Name()] || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		it.Bytes += size.Allocated(info)
		count++
	}
	it.Note = fmt.Sprintf("%d files not used by any model", count)
	return it, count > 0
}

func hasFiles(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
