package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/report"
)

var day1 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// seedState saves three scans a day apart, with free space falling, npm
// growing and a new download, and two clean runs: one after the first scan
// and one after the last.
func seedState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	npm := func(b int64) history.Item {
		return history.Item{Section: "Caches", Label: "npm", Path: home + "/.npm", Bytes: b}
	}
	scans := []history.Snapshot{
		{Time: day1, Free: 100, Total: 1000, Items: []history.Item{npm(10)}},
		{Time: day1.AddDate(0, 0, 1), Free: 90, Total: 1000, Items: []history.Item{npm(10)}},
		{Time: day1.AddDate(0, 0, 2), Free: 70, Total: 1000, Items: []history.Item{
			npm(30), {Section: "Downloads", Label: "big.iso", Path: home + "/Downloads/big.iso", Bytes: 5},
		}, Skipped: []string{home + "/Library/Mail"},
			Run: metrics.Run{Start: day1.AddDate(0, 0, 2), Scanners: []metrics.Scanner{{Name: "Caches", Items: 1, Bytes: 30}}}},
	}
	for _, s := range scans {
		if err := history.SaveScan(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []metrics.Clean{
		{Time: day1.Add(time.Hour), Command: "clean mac", Moved: 1, MovedBytes: 7},
		{Time: day1.AddDate(0, 0, 2).Add(time.Hour), Command: "clean browsers", DryRun: true},
	} {
		if err := history.SaveClean(dir, c); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func connectState(t *testing.T, dir string) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := sdk.NewInMemoryTransports()
	opts := Options{Version: "test", Home: home, StateDir: dir, Log: logging.Discard(), Scan: func(context.Context) (report.Report, error) {
		t.Error("scan called")
		return report.Report{}, nil
	}}
	if _, err := NewServer(opts).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// call calls tool with args and decodes its result into v, failing on an
// error result.
func call(t *testing.T, session *sdk.ClientSession, tool string, args map[string]any, v any) string {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*sdk.TextContent).Text
	if res.IsError {
		t.Fatalf("%s %v: %s", tool, args, text)
	}
	if err := json.Unmarshal([]byte(text), v); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, home) {
		t.Errorf("%s result holds the home folder: %s", tool, text)
	}
	return text
}

func TestHistoryReturnsTrendAndGrowth(t *testing.T) {
	session := connectState(t, seedState(t))
	var got HistoryResult
	call(t, session, "history", map[string]any{"runs": 2}, &got)

	if len(got.Scans) != 2 || got.Scans[0].FreeBytes != 90 || *got.Scans[0].ChangeBytes != -10 ||
		got.Scans[1].FreeBytes != 70 || *got.Scans[1].ChangeBytes != -20 || got.Scans[1].DiskBytes != 1000 {
		t.Errorf("scans = %+v", got.Scans)
	}
	want := []Growth{
		{Section: "Caches", Label: "npm", Path: "~/.npm", GrewBytes: 20},
		{Section: "Downloads", Label: "big.iso", Path: "~/Downloads/big.iso", GrewBytes: 5, New: true},
	}
	if len(got.Growing) != len(want) || got.Growing[0] != want[0] || got.Growing[1] != want[1] {
		t.Errorf("growing = %+v, want %+v", got.Growing, want)
	}
}

func TestHistoryFirstScanHasNoChange(t *testing.T) {
	session := connectState(t, seedState(t))
	var got HistoryResult
	call(t, session, "history", nil, &got)
	if len(got.Scans) != 3 || got.Scans[0].ChangeBytes != nil {
		t.Errorf("scans = %+v, want 3 with no change on the first", got.Scans)
	}
}

func TestHistoryAndStatsWithNoScans(t *testing.T) {
	session := connectState(t, t.TempDir())
	var h HistoryResult
	if text := call(t, session, "history", nil, &h); !strings.Contains(text, `"scans":[]`) || !strings.Contains(text, `"growing":[]`) {
		t.Errorf("history = %s, want empty lists", text)
	}
	var s StatsResult
	if text := call(t, session, "stats", nil, &s); !strings.Contains(text, `"runs":[]`) || !strings.Contains(text, `"cleans":[]`) {
		t.Errorf("stats = %s, want empty lists", text)
	}
}

func TestStatsReturnsRunsSkippedPathsAndCleans(t *testing.T) {
	session := connectState(t, seedState(t))
	var last StatsResult
	call(t, session, "stats", nil, &last)
	if len(last.Runs) != 1 || last.Runs[0].Scanners[0].Bytes != 30 ||
		len(last.Runs[0].SkippedPaths) != 1 || last.Runs[0].SkippedPaths[0] != "~/Library/Mail" {
		t.Errorf("runs = %+v", last.Runs)
	}
	if len(last.Cleans) != 1 || last.Cleans[0].Command != "clean browsers" {
		t.Errorf("cleans since the last scan = %+v", last.Cleans)
	}

	var all StatsResult
	call(t, session, "stats", map[string]any{"runs": 3}, &all)
	if len(all.Runs) != 3 || len(all.Cleans) != 2 || all.Cleans[0].MovedBytes != 7 {
		t.Errorf("runs = %d, cleans = %+v", len(all.Runs), all.Cleans)
	}
}

func TestRunCountsOutOfRangeRejected(t *testing.T) {
	session := connectState(t, seedState(t))
	for _, tool := range []string{"history", "stats"} {
		for _, n := range []int{0, -1, 51} {
			res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: map[string]any{"runs": n}})
			if err != nil {
				t.Fatal(err)
			}
			if text := res.Content[0].(*sdk.TextContent).Text; !res.IsError || !strings.Contains(text, "between 1 and 50") {
				t.Errorf("%s runs=%d: %q (error %v), want rejected", tool, n, text, res.IsError)
			}
		}
		var v any
		call(t, session, tool, map[string]any{"runs": 50}, &v)
	}
}
