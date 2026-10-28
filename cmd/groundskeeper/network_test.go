package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/shaynemeyer/groundskeeper"

// usbPackage is the only package allowed to import go-ios, which talks to
// the iPhone over USB through usbmuxd.
const usbPackage = module + "/internal/iphone"

// mcpPackage is the only package allowed to import the MCP SDK, which also
// holds HTTP transports that gk mcp never uses.
const mcpPackage = module + "/internal/mcp"

// networkImport reports whether an import can open network connections.
func networkImport(path string) bool {
	return path == "net" || strings.HasPrefix(path, "net/") ||
		path == "crypto/tls" || strings.HasPrefix(path, "golang.org/x/net")
}

// TestNoNetworkImports keeps the tool offline: none of its own packages may
// import a network package, only usbPackage may import go-ios, and only
// mcpPackage may import the MCP SDK.
func TestNoNetworkImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", module+"/...").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		pkg, imports := fields[0], fields[1:]
		for _, imp := range imports {
			if networkImport(imp) {
				t.Errorf("%s imports %s", pkg, imp)
			}
			if strings.HasPrefix(imp, "github.com/danielpaulus/go-ios") && pkg != usbPackage {
				t.Errorf("%s imports %s; only %s may talk to the iPhone", pkg, imp, usbPackage)
			}
			if strings.HasPrefix(imp, "github.com/modelcontextprotocol/go-sdk") && pkg != mcpPackage {
				t.Errorf("%s imports %s; only %s may serve MCP", pkg, imp, mcpPackage)
			}
		}
	}
}

// noNetwork is a sandbox profile that kills the process on any outbound
// connection, Unix sockets included.
const noNetwork = `(version 1) (allow default) (deny network-outbound (with send-signal SIGKILL))`

// sandboxed runs the built binary under noNetwork with home as HOME and
// returns its exit code, -1 when a signal killed it.
func sandboxed(t *testing.T, bin, home string, args ...string) int {
	t.Helper()
	cmd := exec.Command("/usr/bin/sandbox-exec", append([]string{"-p", noNetwork, bin}, args...)...)
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Logf("%v: %s", args, out)
		return exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0
}

// TestScanMakesNoConnections runs a full scan where any connection attempt
// kills it. The iPhone read, which connects to usbmuxd, shows the sandbox
// catches this binary's connections.
func TestScanMakesNoConnections(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "groundskeeper")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	if code := sandboxed(t, bin, home, "iphone", "--dry"); code != -1 {
		t.Fatalf("iphone exited %d under the sandbox, want killed: the sandbox isn't catching connections", code)
	}
	if code := sandboxed(t, bin, home, "scan"); code != 0 {
		t.Fatalf("scan exited %d under the sandbox, want 0: it tried to connect", code)
	}
	if _, err := os.Stat(filepath.Join(home, "Reports/groundskeeper/latest.html")); err != nil {
		t.Fatalf("scan wrote no report: %v", err)
	}
	mcpSandboxed(t, bin, home)
}

// mcpRequests initialize a session, list the tools and call scan.
var mcpRequests = []string{
	`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
	`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scan","arguments":{}}}`,
}

// mcpSandboxed runs gk mcp under noNetwork, sends mcpRequests, waits for the
// scan's answer, then disconnects. Any connection attempt kills it.
func mcpSandboxed(t *testing.T, bin, home string) {
	t.Helper()
	cmd := exec.Command("/usr/bin/sandbox-exec", "-p", noNetwork, bin, "mcp")
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stdin.Write([]byte(strings.Join(mcpRequests, "\n") + "\n"))
	lines := bufio.NewScanner(stdout)
	lines.Buffer(nil, 4<<20)
	answered := false
	for !answered && lines.Scan() {
		answered = strings.HasPrefix(lines.Text(), `{"jsonrpc":"2.0","id":3,"result":`)
	}
	stdin.Close()
	err = cmd.Wait()
	if !answered {
		t.Fatalf("gk mcp never answered scan under the sandbox: %v", err)
	}
	if err != nil {
		t.Fatalf("gk mcp under the sandbox: %v; it tried to connect", err)
	}
}
