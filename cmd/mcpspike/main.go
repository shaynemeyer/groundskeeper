// Command mcpspike is the step 16 spike: a throwaway MCP server over stdio
// that answers the plan's three questions. Delete after the spike.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

var spikeLog *log.Logger

type scanArgs struct {
	Top int `json:"top,omitempty" jsonschema:"largest items to return per section; 0 returns all"`
}

type waitArgs struct {
	Seconds int `json:"seconds" jsonschema:"how long to wait before answering"`
}

type item struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Safety  string `json:"safety"`
	Note    string `json:"note,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

type scanResult struct {
	DurationMS     int64          `json:"durationMs"`
	FullDiskAccess string         `json:"fullDiskAccess"`
	Skipped        map[string]int `json:"skippedByReason"`
	SkippedPaths   []string       `json:"skippedPaths"`
	Items          []item         `json:"items"`
}

func main() {
	f, err := os.OpenFile(os.Getenv("SPIKE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(1)
	}
	spikeLog = log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	spikeLog.Printf("start pid=%d ppid=%d", os.Getpid(), os.Getppid())

	server := mcp.NewServer(&mcp.Implementation{Name: "groundskeeper-spike", Version: "0"}, nil)
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	if os.Getenv("SPIKE_WAIT_ONLY") == "" {
		mcp.AddTool(server, &mcp.Tool{Name: "scan", Description: "Scan this Mac for reclaimable disk space.", Annotations: ro}, scanTool)
	} else {
		go localScan()
	}
	mcp.AddTool(server, &mcp.Tool{Name: "wait", Description: "Wait the given number of seconds, then answer. For testing timeouts.", Annotations: ro}, waitTool)
	err = server.Run(context.Background(), &mcp.StdioTransport{})
	spikeLog.Printf("exit err=%v", err)
}

func waitTool(ctx context.Context, req *mcp.CallToolRequest, a waitArgs) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	spikeLog.Printf("wait %ds called", a.Seconds)
	select {
	case <-time.After(time.Duration(a.Seconds) * time.Second):
		spikeLog.Printf("wait %ds answered after %s", a.Seconds, time.Since(start))
	case <-ctx.Done():
		spikeLog.Printf("wait %ds cancelled after %s: %v", a.Seconds, time.Since(start), ctx.Err())
		return nil, nil, ctx.Err()
	}
	return text(fmt.Sprintf("waited %d seconds", a.Seconds)), nil, nil
}

func scanTool(ctx context.Context, req *mcp.CallToolRequest, a scanArgs) (*mcp.CallToolResult, any, error) {
	start := time.Now()
	spikeLog.Printf("scan top=%d called", a.Top)
	home, _ := os.UserHomeDir()
	cfg, err := config.Load(filepath.Join(home, ".config", "groundskeeper", "config.json"))
	if err != nil {
		return nil, nil, err
	}
	res := scanResult{FullDiskAccess: fullDiskAccess(home), Skipped: map[string]int{}}
	for _, s := range scanners(home, cfg) {
		f, err := s.Scan(ctx, home)
		if ctx.Err() != nil {
			spikeLog.Printf("scan cancelled after %s in %s: %v", time.Since(start), s.Name(), ctx.Err())
			return nil, nil, ctx.Err()
		}
		if err != nil {
			spikeLog.Printf("scanner %s failed: %v", s.Name(), err)
			continue
		}
		res.Items = append(res.Items, top(f.Items, a.Top, home)...)
		for _, sk := range f.Skipped {
			res.Skipped[sk.Reason]++
			res.SkippedPaths = append(res.SkippedPaths, tilde(sk.Path, home))
		}
	}
	res.DurationMS = time.Since(start).Milliseconds()
	b, _ := json.Marshal(res)
	spikeLog.Printf("scan answered after %s: fda=%s items=%d skipped=%v bytes=%d",
		time.Since(start), res.FullDiskAccess, len(res.Items), res.Skipped, len(b))
	return text(string(b)), nil, nil
}

// localScan runs a scan under the client's process tree and logs only the
// access probe and skip counts, so nothing about the Mac reaches the client.
func localScan() {
	start := time.Now()
	home, _ := os.UserHomeDir()
	cfg, err := config.Load(filepath.Join(home, ".config", "groundskeeper", "config.json"))
	if err != nil {
		spikeLog.Printf("local scan: config: %v", err)
		return
	}
	skipped := map[string]int{}
	for _, s := range scanners(home, cfg) {
		f, _ := s.Scan(context.Background(), home)
		for _, sk := range f.Skipped {
			skipped[sk.Reason]++
		}
	}
	spikeLog.Printf("local scan done after %s: fda=%s skipped=%v", time.Since(start), fullDiskAccess(home), skipped)
}

func scanners(home string, cfg config.Config) []scan.Scanner {
	m := size.New(2*time.Minute, 6)
	minBytes := cfg.MinSizeMB * 1_000_000
	var roots []string
	for _, r := range cfg.ProjectRoots {
		roots = append(roots, config.Expand(r, home))
	}
	return []scan.Scanner{
		scan.Caches{Size: m, MinBytes: minBytes},
		scan.Models{Size: m, ProjectRoots: roots, MinBytes: minBytes},
		scan.Artifacts{Size: m, Roots: roots, StaleDays: cfg.StaleDays, MinBytes: minBytes},
		scan.Toolchains{Size: m, MinBytes: minBytes},
		scan.Containers{Size: m, MinBytes: minBytes},
		scan.Downloads{Size: m, StaleDays: cfg.StaleDays, MinBytes: minBytes},
		scan.Browsers{Size: m, MinBytes: minBytes},
		scan.Apps{Size: m, UnusedDays: cfg.AppUnusedDays, Exclude: cfg.AppExclude, MinBytes: minBytes},
	}
}

// fullDiskAccess is doctor's probe: ~/Library/Safari is protected.
func fullDiskAccess(home string) string {
	_, err := os.ReadDir(filepath.Join(home, "Library", "Safari"))
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return "granted"
	}
	return "missing"
}

func top(items []scan.Item, n int, home string) []item {
	items = slices.Clone(items)
	slices.SortFunc(items, func(a, b scan.Item) int { return cmp.Compare(b.Bytes, a.Bytes) })
	if n > 0 && len(items) > n {
		items = items[:n]
	}
	var out []item
	for _, it := range items {
		out = append(out, item{it.Section, it.Label, tilde(it.Path, home), it.Bytes, it.Safety, it.Note, strings.ReplaceAll(it.Hint, home, "~")})
	}
	return out
}

func tilde(p, home string) string {
	if rest, ok := strings.CutPrefix(p, home); ok {
		return "~" + rest
	}
	return p
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
