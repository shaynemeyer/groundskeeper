package mcp

import (
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

const maxRuns = 50

const historyDescription = `Show how free disk space changed over recent scans, and the items that grew ` +
	`most between the last two scans. Reads saved scans; doesn't scan. Read-only.`

const statsDescription = `Show how recent scans went: each scanner's time, items, size, skipped and ` +
	`timed-out paths and errors, the paths that couldn't be read, and the clean commands run since. ` +
	`Reads saved scans; doesn't scan. Read-only.`

// RunsInput is the history and stats tools' argument.
type RunsInput struct {
	Runs *int `json:"runs,omitempty" jsonschema:"how many recent scans to include, 1 to 50"`
}

// HistoryResult is what the history tool returns.
type HistoryResult struct {
	Scans   []ScanPoint `json:"scans"`
	Growing []Growth    `json:"growing"` // between the last two scans, largest first
}

// ScanPoint is one saved scan's disk space.
type ScanPoint struct {
	Time        time.Time `json:"time"`
	FreeBytes   int64     `json:"freeBytes"`
	ChangeBytes *int64    `json:"changeBytes,omitempty"` // free space change from the scan before
	DiskBytes   int64     `json:"diskBytes"`
}

// Growth is an item that grew between the last two scans.
type Growth struct {
	Section   string `json:"section"`
	Label     string `json:"label"`
	Path      string `json:"path,omitempty"`
	GrewBytes int64  `json:"grewBytes"`
	New       bool   `json:"new,omitempty"` // not in the scan before
}

// StatsResult is what the stats tool returns.
type StatsResult struct {
	Runs   []RunStats      `json:"runs"`
	Cleans []metrics.Clean `json:"cleans"` // clean commands since the oldest run listed
}

// RunStats is one scan's metrics and the paths it couldn't read.
type RunStats struct {
	metrics.Run
	SkippedPaths []string `json:"skippedPaths"`
}

// runs returns the requested count, def when it's left out.
func runs(in RunsInput, def int) (int, error) {
	if in.Runs == nil {
		return def, nil
	}
	if *in.Runs < 1 || *in.Runs > maxRuns {
		return 0, fmt.Errorf("runs must be between 1 and %d", maxRuns)
	}
	return *in.Runs, nil
}

func scans(opts Options) ([]history.Snapshot, error) {
	s, err := history.Scans(opts.StateDir)
	if err != nil {
		return nil, fmt.Errorf("reading saved scans failed: %s", logging.KindOf(err))
	}
	return s, nil
}

func historyOf(opts Options, in RunsInput) (*sdk.CallToolResult, HistoryResult, error) {
	n, err := runs(in, 10)
	if err != nil {
		return nil, HistoryResult{}, err
	}
	all, err := scans(opts)
	if err != nil {
		return nil, HistoryResult{}, err
	}
	res := HistoryResult{Scans: []ScanPoint{}, Growing: []Growth{}}
	for i := max(0, len(all)-n); i < len(all); i++ {
		p := ScanPoint{Time: all[i].Time, FreeBytes: all[i].Free, DiskBytes: all[i].Total}
		if i > 0 {
			change := all[i].Free - all[i-1].Free
			p.ChangeBytes = &change
		}
		res.Scans = append(res.Scans, p)
	}
	if len(all) >= 2 {
		for _, c := range history.Compare(&all[len(all)-2], all[len(all)-1]).Growing {
			res.Growing = append(res.Growing, Growth{
				Section: c.Section, Label: c.Label, Path: tilde(c.Path, opts.Home), GrewBytes: c.Delta, New: c.New,
			})
		}
	}
	return nil, res, nil
}

func statsOf(opts Options, in RunsInput) (*sdk.CallToolResult, StatsResult, error) {
	n, err := runs(in, 1)
	if err != nil {
		return nil, StatsResult{}, err
	}
	all, err := scans(opts)
	if err != nil {
		return nil, StatsResult{}, err
	}
	res := StatsResult{Runs: []RunStats{}, Cleans: []metrics.Clean{}}
	recent := all[max(0, len(all)-n):]
	for _, s := range recent {
		r := RunStats{Run: s.Run, SkippedPaths: []string{}}
		for _, p := range s.Skipped {
			r.SkippedPaths = append(r.SkippedPaths, tilde(p, opts.Home))
		}
		res.Runs = append(res.Runs, r)
	}
	if len(recent) > 0 {
		cleans, err := history.Cleans(opts.StateDir, recent[0].Time)
		if err != nil {
			return nil, StatsResult{}, fmt.Errorf("reading clean runs failed: %s", logging.KindOf(err))
		}
		res.Cleans = append(res.Cleans, cleans...)
	}
	return nil, res, nil
}
