package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

func (c *mcpClient) scan() (text string, result struct {
	Items []struct {
		Path, Hint string
	}
	Skipped []struct{ Reason string }
}) {
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
	text = res.Content[0].Text
	if res.IsError {
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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "scan" || !tools.Tools[0].Annotations.ReadOnlyHint {
		t.Fatalf("tools = %+v, want only scan, read-only", tools.Tools)
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
	if !slices.Equal(added, []string{"Library/Logs/groundskeeper.log"}) {
		t.Errorf("files added = %v, want only the log", added)
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
