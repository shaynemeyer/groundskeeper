package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/report"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
)

const home = "/Users/someone"

// connect starts a server on in-memory transports and returns a client
// session to it.
func connect(t *testing.T, s Scan) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := sdk.NewInMemoryTransports()
	if _, err := NewServer(Options{Version: "test", Home: home, StateDir: t.TempDir(), Scan: s, Log: logging.Discard()}).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func sampleReport() report.Report {
	cache := home + "/Library/Caches/Some App"
	return report.Report{
		Disk: scan.Disk{Free: 100, Total: 1000},
		Sections: []report.Section{{Name: "Caches", Items: []scan.Item{{
			Section: "Caches", Label: "Some App", Path: cache, Bytes: 42, Safety: scan.Safe,
			Hint: "rm -rf " + scan.ShellQuote(cache),
		}}}},
		Skipped: []size.Skip{{Path: home + "/Library/Mail", Reason: "permission"}},
	}
}

func callScan(t *testing.T, session *sdk.ClientSession) (*sdk.CallToolResult, string) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	return res, res.Content[0].(*sdk.TextContent).Text
}

func TestToolsAreScanHistoryStatsAndReadOnly(t *testing.T) {
	session := connect(t, func(context.Context) (report.Report, error) { return report.Report{}, nil })
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s isn't marked read-only", tool.Name)
		}
	}
	if fmt.Sprint(names) != "[history scan stats]" {
		t.Errorf("tools = %v, want [history scan stats]", names)
	}
}

func TestScanResultWritesHomeAsTilde(t *testing.T) {
	session := connect(t, func(context.Context) (report.Report, error) { return sampleReport(), nil })
	res, text := callScan(t, session)
	if res.IsError {
		t.Fatalf("scan failed: %s", text)
	}
	if strings.Contains(text, home) {
		t.Errorf("result holds the home folder: %s", text)
	}
	var got ScanResult
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	it := got.Items[0]
	if it.Path != "~/Library/Caches/Some App" || it.Hint != "rm -rf ~/'Library/Caches/Some App'" {
		t.Errorf("item = %+v", it)
	}
	if got.Skipped[0].Path != "~/Library/Mail" || got.Advice == "" {
		t.Errorf("skipped = %+v, advice = %q", got.Skipped, got.Advice)
	}
	if got.Reclaimable != 42 || got.DiskFree != 100 || got.DiskTotal != 1000 {
		t.Errorf("totals = %+v", got)
	}
}

// TestTildeHintStillExpands runs rewritten hints' arguments through the
// shell, as a pasted hint would be, and checks they name the same paths.
func TestTildeHintStillExpands(t *testing.T) {
	for _, path := range []string{home, home + "/a b", home + "/it's", home + "/'lead", home + "/$(x)"} {
		hint := tildeHint("printf %s "+scan.ShellQuote(path), home)
		if strings.Contains(hint, home) {
			t.Errorf("%q still holds home", hint)
		}
		cmd := exec.Command("/bin/sh", "-c", hint)
		cmd.Env = []string{"HOME=" + home}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%q: %v", hint, err)
		}
		if string(out) != path {
			t.Errorf("%q printed %q, want %q", hint, out, path)
		}
	}
}

func TestTildeLeavesOtherPathsAlone(t *testing.T) {
	for _, p := range []string{"/Applications/Some.app", home + "2/x", "relative"} {
		if got := tilde(p, home); got != p {
			t.Errorf("tilde(%q) = %q", p, got)
		}
		hint := "rm -rf " + scan.ShellQuote(p)
		if got := tildeHint(hint, home); got != hint {
			t.Errorf("tildeHint(%q) = %q", hint, got)
		}
	}
}

func TestScanErrorReportsKindOnly(t *testing.T) {
	session := connect(t, func(context.Context) (report.Report, error) {
		return report.Report{}, &fs.PathError{Op: "open", Path: home + "/secret", Err: fs.ErrPermission}
	})
	res, text := callScan(t, session)
	if !res.IsError {
		t.Fatal("want an error result")
	}
	if strings.Contains(text, home) || !strings.Contains(text, "permission") {
		t.Errorf("error text = %q, want the kind without the path", text)
	}
}

func TestBusyErrorReachesClient(t *testing.T) {
	session := connect(t, func(context.Context) (report.Report, error) { return report.Report{}, ErrBusy })
	res, text := callScan(t, session)
	if !res.IsError || text != ErrBusy.Error() {
		t.Errorf("result = %q (error %v), want %q", text, res.IsError, ErrBusy)
	}
}

func TestEachToolCallLogsOneLine(t *testing.T) {
	var buf strings.Builder
	var scanErr error
	opts := Options{Version: "test", Home: home, StateDir: t.TempDir(), Log: logging.Text(&buf, "mcp"),
		Scan: func(context.Context) (report.Report, error) { return sampleReport(), scanErr }}
	ctx := context.Background()
	serverT, clientT := sdk.NewInMemoryTransports()
	if _, err := NewServer(opts).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	calls := []struct {
		tool    string
		args    map[string]any
		scanErr error
		want    []string
	}{
		{"scan", nil, nil, []string{"level=INFO", "tool=scan", "outcome=ok"}},
		{"scan", nil, ErrBusy, []string{"level=WARN", "tool=scan", "outcome=busy"}},
		{"scan", nil, &fs.PathError{Op: "open", Path: home + "/secret", Err: fs.ErrPermission}, []string{"level=WARN", "tool=scan", "outcome=failed"}},
		{"history", nil, nil, []string{"level=INFO", "tool=history", "outcome=ok"}},
		{"stats", map[string]any{"runs": 0}, nil, []string{"level=WARN", "tool=stats", "outcome=rejected"}},
	}
	for _, c := range calls {
		scanErr = c.scanErr
		buf.Reset()
		if _, err := session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.args}); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		want := append([]string{`msg="mcp tool call"`, "duration_ms="}, c.want...)
		if len(lines) != 1 || slices.ContainsFunc(want, func(w string) bool { return !strings.Contains(lines[0], w) }) {
			t.Errorf("%s %v logged %q, want one line with %q", c.tool, c.args, lines, want)
		}
		if strings.Contains(buf.String(), home) {
			t.Errorf("%s logged the home folder: %s", c.tool, buf.String())
		}
	}
}

func TestScanListsFirstSkippedPathsAndCountsAll(t *testing.T) {
	rep := sampleReport()
	rep.Skipped = nil
	for i := range 25 {
		rep.Skipped = append(rep.Skipped, size.Skip{Path: fmt.Sprintf("%s/slow/%d", home, i), Reason: "timeout"})
	}
	rep.Skipped = append(rep.Skipped, size.Skip{Path: home + "/Library/Mail", Reason: "permission"})
	session := connect(t, func(context.Context) (report.Report, error) { return rep, nil })
	_, text := callScan(t, session)
	var got ScanResult
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Skipped) != maxSkipped || got.SkippedTotal != 26 ||
		got.SkippedByReason["timeout"] != 25 || got.SkippedByReason["permission"] != 1 {
		t.Errorf("listed %d, total %d, by reason %v; want %d, 26, 25 timeout and 1 permission",
			len(got.Skipped), got.SkippedTotal, got.SkippedByReason, maxSkipped)
	}
	if got.Advice == "" {
		t.Error("no Full Disk Access advice for a permission skip past the first 20")
	}
}
