package scan

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

const pycache = "__pycache__"

// Artifacts finds build output in project folders: node_modules, venvs,
// Rust target, .next, .NET bin and obj, and __pycache__ per project.
type Artifacts struct {
	Size      *size.Measurer
	Roots     []string // absolute project roots
	Exclude   []string // absolute paths never descended into
	StaleDays int
	MinBytes  int64
}

type artifact struct {
	path, kind, project, root string
}

// Name returns the scanner name.
func (a Artifacts) Name() string { return "Build artifacts" }

// Scan walks each root and measures the artifact folders it finds.
func (a Artifacts) Scan(ctx context.Context, _ string) (Findings, error) {
	var f Findings
	var found []artifact
	for _, root := range a.Roots {
		found = append(found, a.walk(root, &f)...)
	}

	paths := make([]string, len(found))
	for i, art := range found {
		paths[i] = art.path
	}
	results := a.Size.MeasureAll(ctx, paths)

	activities := map[string]activity{}
	pycaches := map[string]*pycacheTotal{}
	for i, art := range found {
		f.Skipped = append(f.Skipped, results[i].Skipped...)
		bytes := results[i].Bytes
		if art.kind == pycache {
			addPycache(pycaches, art, bytes)
			continue
		}
		act, ok := activities[art.project]
		if !ok {
			act = projectActivity(ctx, art.project)
			activities[art.project] = act
		}
		it, broken := a.item(art, bytes, act)
		if it.Bytes >= a.MinBytes || broken {
			f.Items = append(f.Items, it)
		}
	}
	for _, p := range pycaches {
		if p.bytes >= a.MinBytes {
			f.Items = append(f.Items, p.item())
		}
	}
	slices.SortFunc(f.Items, func(x, y Item) int { return cmp.Compare(y.Bytes, x.Bytes) })
	return f, nil
}

func (a Artifacts) walk(root string, f *Findings) []artifact {
	var found []artifact
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				f.Skipped = append(f.Skipped, size.NewSkip(path, err))
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() || path == root {
			return nil
		}
		if d.Name() == ".git" || slices.Contains(a.Exclude, path) {
			return filepath.SkipDir
		}
		kind := artifactKind(path, d.Name())
		if kind == "" {
			return nil
		}
		found = append(found, artifact{path: path, kind: kind, project: projectOf(root, path), root: root})
		return filepath.SkipDir
	})
	return found
}

// artifactKind returns the kind of artifact folder at path, or "".
func artifactKind(path, name string) string {
	parent := filepath.Dir(path)
	switch name {
	case "node_modules", ".next", pycache:
		return name
	case ".venv", "venv":
		if exists(filepath.Join(path, "pyvenv.cfg")) {
			return "venv"
		}
	case "target":
		if exists(filepath.Join(parent, "Cargo.toml")) {
			return "target"
		}
	case "bin", "obj":
		if hasCsproj(parent) {
			return name
		}
	}
	return ""
}

// projectOf returns the top-level folder under root that holds path.
func projectOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return root
	}
	first, _, nested := strings.Cut(rel, string(filepath.Separator))
	if !nested {
		return root
	}
	return filepath.Join(root, first)
}

// activity is how long ago a project was last worked on.
type activity struct {
	days int // -1 when nothing was found
	how  string
}

func projectActivity(ctx context.Context, project string) activity {
	t, how := lastActivity(ctx, project)
	if t.IsZero() {
		return activity{days: -1}
	}
	return activity{days: int(time.Since(t).Hours() / 24), how: how}
}

func (act activity) note() string {
	if act.days < 0 {
		return "no activity found"
	}
	return fmt.Sprintf("%s %d days ago", act.how, act.days)
}

// item builds the report row for art and reports whether it is a broken venv.
func (a Artifacts) item(art artifact, bytes int64, act activity) (Item, bool) {
	it := Item{
		Section: "Build artifacts",
		Label:   label(art.root, art.path),
		Path:    art.path,
		Bytes:   bytes,
		Safety:  Review,
		Hint:    "rm -rf " + ShellQuote(art.path),
		Note:    act.note(),
		Trash:   art.path,
	}
	if act.days < 0 || act.days >= a.StaleDays {
		it.Safety = Safe
	}
	if art.kind == "target" {
		it.Hint = "cargo clean --manifest-path " + ShellQuote(filepath.Join(filepath.Dir(art.path), "Cargo.toml"))
	}
	if art.kind != "venv" {
		return it, false
	}
	home, broken := brokenVenv(art.path)
	if broken {
		it.Safety = Safe
		it.Note = fmt.Sprintf("broken: its Python at %s no longer exists", home)
	}
	return it, broken
}

// pycacheTotal adds up a project's __pycache__ folders into one row.
type pycacheTotal struct {
	art     artifact
	bytes   int64
	folders int
}

func addPycache(totals map[string]*pycacheTotal, art artifact, bytes int64) {
	p, ok := totals[art.project]
	if !ok {
		p = &pycacheTotal{art: art}
		totals[art.project] = p
	}
	p.bytes += bytes
	p.folders++
}

func (p pycacheTotal) item() Item {
	project := p.art.project
	return Item{
		Section: "Build artifacts",
		Label:   label(p.art.root, filepath.Join(project, pycache)),
		Path:    project,
		Bytes:   p.bytes,
		Safety:  Safe,
		Hint:    "find " + ShellQuote(project) + " -name __pycache__ -type d -prune -exec rm -rf {} +",
		Note:    fmt.Sprintf("%d folders", p.folders),
	}
}

func label(root, path string) string {
	rel, err := filepath.Rel(filepath.Dir(root), path)
	if err != nil {
		return path
	}
	return rel
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasCsproj(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e os.DirEntry) bool {
		return strings.HasSuffix(e.Name(), ".csproj")
	})
}
