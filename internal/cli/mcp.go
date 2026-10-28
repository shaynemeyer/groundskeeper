package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/mcp"
	"github.com/shaynemeyer/groundskeeper/internal/report"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// mcpBudget is how long an MCP scan may take. Claude Desktop gives up on a
// tool call after 60 seconds.
var mcpBudget = 45 * time.Second

func newMCPCmd(global *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "mcp",
		GroupID: groupReport,
		Short:   "Serve read-only results to an MCP client over stdio",
		Long: `Lets an AI assistant answer "what's using my disk?". An MCP client such as
Claude Desktop, or ollmcp with a local Ollama model, starts gk mcp and talks to
it on stdin and stdout. It offers one tool, scan, which runs a scan like
gk scan --dry and returns each item's path, size, safety tag and suggested
command. A scan stops after 45 seconds and lists what it didn't measure.

Privacy: Groundskeeper sends nothing anywhere, but the client passes results
to its model. With Claude Desktop they go to Anthropic: paths that name your
projects, apps and Downloads files leave this Mac. With a local Ollama model
they don't. Paths under your home folder are written as ~.

Full Disk Access: the client starts this binary as its own process, so it
needs Full Disk Access granted to ~/bin/groundskeeper, the same grant the
schedule uses. The client app's access doesn't apply.

Safety: read-only. It writes no report, snapshot or file other than the log,
and offers no clean commands.

See also: gk help privacy, gk scan`,
		Example: `  # Clients start it themselves; this is the command they run.
  # In Claude Desktop's claude_desktop_config.json, use the full path:
  #   "groundskeeper": {"command": "<full path of ~/bin/groundskeeper>", "args": ["mcp"]}
  gk mcp`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd, global)
		},
	}
}

// runMCP serves until the client disconnects. Log lines go only to the log
// file, never to stdout or stderr, even with --verbose.
func runMCP(cmd *cobra.Command, global *globalOptions) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "Logs", "groundskeeper.log")
	log, closeLog, err := logging.Open(path, logging.Options{Level: cfg.LogLevel, Cmd: "mcp"})
	if err != nil {
		return err
	}
	defer closeLog.Close()
	log.Info("mcp started")
	scanFn := func(ctx context.Context) (report.Report, error) { return mcpScan(ctx, log, home, cfg) }
	err = mcp.Serve(cmd.Context(), cmd.Root().Version, home, scanFn, cmd.InOrStdin(), cmd.OutOrStdout())
	log.Info("mcp stopped")
	return err
}

// mcpScan measures like gk scan --dry within mcpBudget, writing no files.
func mcpScan(ctx context.Context, log *logging.Logger, home string, cfg config.Config) (report.Report, error) {
	unlock, err := lockScan(home)
	if err != nil {
		log.Warn("mcp scan refused", logging.ErrKind(err))
		return report.Report{}, err
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(ctx, mcpBudget)
	defer cancel()
	start := time.Now()
	log.Info("mcp scan started")
	disk, err := scan.DiskUsage(home)
	if err != nil {
		log.Error("mcp scan failed", logging.ErrKind(err))
		return report.Report{}, err
	}
	rep := report.Report{Generated: start, Disk: disk}
	rep.Sections, rep.Skipped, _ = runScanners(ctx, log, newScanners(home, cfg, size.New(folderTimeout, sizeWorkers)), home, &ui.Spinner{})
	log.Info("mcp scan finished", logging.Duration(time.Since(start)), logging.Bytes(rep.Reclaimable()),
		logging.Count("skipped", len(rep.Skipped)))
	return rep, nil
}

// lockScan takes the scan lock in the state folder, which every gk mcp
// process shares; Claude Desktop starts two. The kernel releases it if the
// process dies.
func lockScan(home string) (unlock func(), err error) {
	dir := history.Dir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "mcp.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, mcp.ErrBusy
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}
