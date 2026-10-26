package cli

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

type logsOptions struct {
	level  string
	run    string
	cmd    string
	lines  int
	follow bool
	json   bool
}

var levelStyles = map[slog.Level]lipgloss.Style{
	slog.LevelDebug: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	slog.LevelWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
	slog.LevelError: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
}

func newLogsCmd(global *globalOptions) *cobra.Command {
	opts := &logsOptions{level: "debug", lines: 50}
	cmd := &cobra.Command{
		Use:     "logs",
		GroupID: groupReport,
		Short:   "Show recent log entries, with filters",
		Long: `Shows entries from ~/Library/Logs/groundskeeper.log: each run's start and
end, every scanner's result, skipped paths, and every clean action. Entries
only ever hold paths, sizes, counts, durations, IDs and error kinds.

Safety: read-only.`,
		Example: `  # The last 50 entries
  gk logs

  # Only warnings and errors from the most recent run
  gk logs --level warn --run last

  # Every clean action, then keep watching for new ones
  gk logs --cmd clean --follow

  # Raw JSON lines
  gk logs --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogs(cmd, global, opts)
		},
	}
	cmd.Flags().StringVar(&opts.level, "level", opts.level, "lowest level to show: debug, info, warn or error")
	cmd.Flags().StringVar(&opts.run, "run", "", `only one run: a run ID, or "last"`)
	cmd.Flags().StringVar(&opts.cmd, "cmd", "", `only commands starting with this, such as "clean"`)
	cmd.Flags().IntVarP(&opts.lines, "lines", "n", opts.lines, "how many of the newest entries to show")
	cmd.Flags().BoolVarP(&opts.follow, "follow", "f", false, "keep printing new entries until Ctrl-C")
	cmd.Flags().BoolVar(&opts.json, "json", false, "print the raw JSON lines")
	return cmd
}

func runLogs(cmd *cobra.Command, global *globalOptions, opts *logsOptions) error {
	home, _, err := loadConfig(global)
	if err != nil {
		return err
	}
	f := logging.Filter{Run: opts.run, Cmd: opts.cmd, Limit: opts.lines}
	if err := f.MinLevel.UnmarshalText([]byte(opts.level)); err != nil {
		return fmt.Errorf("--level: %w", err)
	}
	path := filepath.Join(home, "Library", "Logs", "groundskeeper.log")
	entries, offset, err := logging.Read(path, f)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	w := cmd.OutOrStdout()
	plain := ui.Plain(w)
	show := func(e logging.Entry) { fmt.Fprintln(w, formatEntry(e, opts.json, plain)) }
	for _, e := range entries {
		show(e)
	}
	if !opts.follow {
		return nil
	}
	if f.Run == "last" && len(entries) > 0 {
		f.Run = entries[len(entries)-1].RunID
	}
	return logging.Follow(cmd.Context(), path, offset, f, show)
}

func formatEntry(e logging.Entry, raw, plain bool) string {
	if raw {
		return e.Raw
	}
	level := fmt.Sprintf("%-5s", e.Level.String())
	if style, ok := levelStyles[e.Level]; ok && !plain {
		level = style.Render(level)
	}
	line := fmt.Sprintf("%s  %s  %-14s  %s", e.Time.Local().Format("2006-01-02 15:04:05"), level, e.Cmd, e.Msg)
	if len(e.Attrs) > 0 {
		line += "  " + strings.Join(e.Attrs, " ")
	}
	return line
}
