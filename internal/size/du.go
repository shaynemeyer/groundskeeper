// Package size measures disk usage the way the disk sees it: allocated
// blocks, symlinks skipped and hardlinks counted once.
package size

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Skip records a path that could not be measured and why.
type Skip struct {
	Path   string
	Reason string // "permission", "timeout" or "error"
}

// Result is the measured size of one folder.
type Result struct {
	Path    string
	Bytes   int64
	Skipped []Skip
}

type inode struct {
	dev uint64
	ino uint64
}

// Measurer sizes folders. Hardlinked files are counted once across every
// folder measured by the same Measurer.
type Measurer struct {
	Timeout time.Duration
	Workers int

	mu   sync.Mutex
	seen map[inode]struct{}
}

// New returns a Measurer with a per-folder timeout and a worker count.
func New(timeout time.Duration, workers int) *Measurer {
	return &Measurer{Timeout: timeout, Workers: workers, seen: map[inode]struct{}{}}
}

// Measure returns the allocated size of root. A missing root measures zero.
func (m *Measurer) Measure(ctx context.Context, root string) Result {
	ctx, cancel := context.WithTimeout(ctx, m.Timeout)
	defer cancel()

	res := Result{Path: root}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			res.Skipped = append(res.Skipped, Skip{Path: root, Reason: "timeout"})
			return filepath.SkipAll
		}
		if err != nil {
			return m.skip(&res, path, d, err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return m.skip(&res, path, d, err)
		}
		res.Bytes += m.allocated(info)
		return nil
	})
	return res
}

// MeasureAll measures each root on the worker pool, keeping input order.
func (m *Measurer) MeasureAll(ctx context.Context, roots []string) []Result {
	results := make([]Result, len(roots))
	sem := make(chan struct{}, m.Workers)
	var wg sync.WaitGroup
	for i, root := range roots {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = m.Measure(ctx, root)
		})
	}
	wg.Wait()
	return results
}

// NewSkip records path as skipped because of err.
func NewSkip(path string, err error) Skip {
	reason := "error"
	if errors.Is(err, fs.ErrPermission) {
		reason = "permission"
	}
	return Skip{Path: path, Reason: reason}
}

func (m *Measurer) skip(res *Result, path string, d fs.DirEntry, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	res.Skipped = append(res.Skipped, NewSkip(path, err))
	if d != nil && d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func (m *Measurer) allocated(info fs.FileInfo) int64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	if st.Nlink > 1 && !info.IsDir() {
		key := inode{dev: uint64(st.Dev), ino: st.Ino}
		m.mu.Lock()
		_, dup := m.seen[key]
		m.seen[key] = struct{}{}
		m.mu.Unlock()
		if dup {
			return 0
		}
	}
	return st.Blocks * 512
}
