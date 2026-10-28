package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/mcp"
)

// mcpClient speaks raw JSON-RPC to gk mcp through OS pipes, as a client
// app would, and checks every line gk mcp writes to stdout. OS pipes are
// buffered like a real client's, so a stray stdout line is read and caught
// rather than blocking.
type mcpClient struct {
	t      *testing.T
	in     *os.File
	out    *bufio.Scanner
	id     int
	stderr *bytes.Buffer
	done   chan error
}

func startMCP(t *testing.T, args ...string) *mcpClient {
	t.Helper()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { inR.Close(); outR.Close() })
	c := &mcpClient{t: t, in: inW, out: bufio.NewScanner(outR), stderr: &bytes.Buffer{}, done: make(chan error, 1)}
	c.out.Buffer(nil, 4<<20)
	root := NewRoot()
	root.SetArgs(append([]string{"mcp"}, args...))
	root.SetIn(inR)
	root.SetOut(outW)
	root.SetErr(c.stderr)
	go func() {
		err := root.ExecuteContext(context.Background())
		outW.Close()
		c.done <- err
	}()
	c.call("initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0"},
	})
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c
}

func (c *mcpClient) send(msg map[string]any) {
	c.t.Helper()
	line, _ := json.Marshal(msg)
	if _, err := c.in.Write(append(line, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// next reads one stdout line and fails unless it's a JSON-RPC message.
func (c *mcpClient) next() (id int, result json.RawMessage, rpcErr string, ok bool) {
	c.t.Helper()
	if !c.out.Scan() {
		return 0, nil, "", false
	}
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(c.out.Bytes(), &msg); err != nil || msg.JSONRPC != "2.0" {
		c.t.Fatalf("stdout line isn't a protocol message: %q", c.out.Text())
	}
	if msg.Error != nil {
		rpcErr = msg.Error.Message
	}
	return msg.ID, msg.Result, rpcErr, true
}

func (c *mcpClient) call(method string, params any) json.RawMessage {
	c.t.Helper()
	c.id++
	c.send(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	for {
		id, result, rpcErr, ok := c.next()
		if !ok {
			c.t.Fatalf("%s: no response", method)
		}
		if id != c.id {
			continue
		}
		if rpcErr != "" {
			c.t.Fatalf("%s: %s", method, rpcErr)
		}
		return result
	}
}

// close disconnects like a client quitting, checks the rest of stdout and
// returns stderr.
func (c *mcpClient) close() string {
	c.t.Helper()
	c.in.Close()
	for {
		if _, _, _, ok := c.next(); !ok {
			break
		}
	}
	if err := <-c.done; err != nil {
		c.t.Fatalf("gk mcp: %v", err)
	}
	return c.stderr.String()
}

// tool calls a tool with no arguments and decodes its structured result
// into v.
func (c *mcpClient) tool(name string, v any) {
	c.t.Helper()
	var res struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(c.call("tools/call", map[string]any{"name": name, "arguments": map[string]any{}}), &res); err != nil {
		c.t.Fatal(err)
	}
	if err := json.Unmarshal(res.StructuredContent, v); err != nil {
		c.t.Fatal(err)
	}
}

// callScan calls the scan tool and returns its text and whether it's an
// error result.
func (c *mcpClient) callScan() (text string, isError bool) {
	c.t.Helper()
	var res struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(c.call("tools/call", map[string]any{"name": "scan", "arguments": map[string]any{}}), &res); err != nil {
		c.t.Fatal(err)
	}
	return res.Content[0].Text, res.IsError
}

func (c *mcpClient) scan() (text string, result struct {
	Items []struct {
		Path, Hint string
	}
	Skipped []struct{ Reason string }
}) {
	c.t.Helper()
	text, isError := c.callScan()
	if isError {
		c.t.Fatalf("scan failed: %s", text)
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		c.t.Fatal(err)
	}
	return text, result
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestMCPServesScanAndWritesOnlyTheLog(t *testing.T) {
	home := fakeHome(t)
	t.Setenv("GITHUB_TOKEN", marker)
	before := listFiles(t, home)

	c := startMCP(t, "--verbose")
	var tools struct {
		Tools []struct {
			Name        string `json:"name"`
			Annotations struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(c.call("tools/list", map[string]any{}), &tools); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s isn't marked read-only", tool.Name)
		}
	}
	if !slices.Equal(names, []string{"history", "scan", "stats"}) {
		t.Fatalf("tools = %v, want history, scan and stats", names)
	}
	text, res := c.scan()
	if stderr := c.close(); stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}

	if strings.Contains(text, home) || strings.Contains(text, marker) {
		t.Errorf("result holds the home folder or the marker: %s", text)
	}
	if !slices.ContainsFunc(res.Items, func(it struct{ Path, Hint string }) bool { return it.Path == "~/.npm" }) {
		t.Errorf("no ~/.npm item in %s", text)
	}
	after := listFiles(t, home)
	added := slices.DeleteFunc(slices.Clone(after), func(f string) bool { return slices.Contains(before, f) })
	if !slices.Equal(added, []string{".local/state/groundskeeper/mcp.lock", "Library/Logs/groundskeeper.log"}) {
		t.Errorf("files added = %v, want only the lock and the log", added)
	}
	log, err := os.ReadFile(filepath.Join(home, "Library/Logs/groundskeeper.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(log, []byte(`"cmd":"mcp"`)) || bytes.Contains(log, []byte(marker)) {
		t.Errorf("log lacks cmd mcp or holds the marker:\n%s", log)
	}
}

func TestMCPScanStopsAtBudget(t *testing.T) {
	fakeHome(t)
	mcpBudget = 0
	t.Cleanup(func() { mcpBudget = 45 * time.Second })

	c := startMCP(t)
	_, res := c.scan()
	c.close()
	if !slices.ContainsFunc(res.Skipped, func(s struct{ Reason string }) bool { return s.Reason == "timeout" }) {
		t.Errorf("skipped = %+v, want folders listed as timeout", res.Skipped)
	}
}

func TestMCPRefusesScanWhileAnotherRuns(t *testing.T) {
	home := fakeHome(t)
	unlock, err := lockScan(home)
	if err != nil {
		t.Fatal(err)
	}
	c := startMCP(t)
	text, isError := c.callScan()
	if !isError || !strings.Contains(text, "already running") {
		t.Errorf("scan while locked = %q (error %v), want the busy error", text, isError)
	}
	unlock()
	c.scan()
	c.close()
}

// TestHelperHoldLock is not a test: TestMCPLockSharedAcrossProcesses runs
// the test binary with it to hold the lock from another process.
func TestHelperHoldLock(t *testing.T) {
	home := os.Getenv("GK_HOLD_LOCK_HOME")
	if home == "" {
		t.Skip("helper process only")
	}
	if _, err := lockScan(home); err != nil {
		t.Fatal(err)
	}
	os.Stdout.WriteString("locked\n")
	io.ReadAll(os.Stdin)
}

func TestMCPLockSharedAcrossProcesses(t *testing.T) {
	home := fakeHome(t)
	child := exec.Command(os.Args[0], "-test.run=^TestHelperHoldLock$")
	child.Env = append(os.Environ(), "GK_HOLD_LOCK_HOME="+home)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if line, _ := bufio.NewReader(stdout).ReadString('\n'); line != "locked\n" {
		t.Fatalf("helper said %q", line)
	}
	if _, err := lockScan(home); !errors.Is(err, mcp.ErrBusy) {
		t.Fatalf("lock held by another process: err = %v, want ErrBusy", err)
	}
	child.Process.Kill()
	child.Wait()
	unlock, err := lockScan(home)
	if err != nil {
		t.Fatalf("lock after the holder died: %v", err)
	}
	unlock()
}

func TestMCPHistoryAndStatsReadSavedScans(t *testing.T) {
	fakeHome(t)
	run(t, "scan")
	c := startMCP(t)
	var hist struct {
		Scans []struct {
			FreeBytes int64 `json:"freeBytes"`
		} `json:"scans"`
	}
	c.tool("history", &hist)
	var stats struct {
		Runs []struct {
			Command string `json:"command"`
		} `json:"runs"`
	}
	c.tool("stats", &stats)
	c.close()
	if len(hist.Scans) != 1 || hist.Scans[0].FreeBytes == 0 {
		t.Errorf("history scans = %+v, want the one saved scan", hist.Scans)
	}
	if len(stats.Runs) != 1 || stats.Runs[0].Command != "scan" {
		t.Errorf("stats runs = %+v, want the saved scan's run", stats.Runs)
	}
}
