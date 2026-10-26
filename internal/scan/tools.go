package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// toolDirs are checked after $PATH, because launchd jobs run with only
// /usr/bin:/bin:/usr/sbin:/sbin.
var toolDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/podman/bin"}

// findTool returns the absolute path of an installed command, or "".
func findTool(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, dir := range toolDirs {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return ""
}

// Runner runs a command and returns its stdout. Tests replace it.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// runTool finds name and runs it with args, never through a shell.
func runTool(ctx context.Context, name string, args ...string) ([]byte, error) {
	path := findTool(name)
	if path == "" {
		return nil, exec.ErrNotFound
	}
	cmd := exec.CommandContext(ctx, path, args...)
	// Keep brew from updating itself, which would reach the network.
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1")
	return cmd.Output()
}
