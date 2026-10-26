package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const groupClean = "clean"

func newCleanCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clean",
		GroupID: groupClean,
		Short:   "Clean up what the report suggests (dry run unless --apply)",
		Long: `Commands that remove things the report suggests. Nothing is ever
cleaned on the schedule; you run these yourself.

Safety: every clean command is a dry run unless you add --apply, and every
action is written to ~/Library/Logs/groundskeeper.log.`,
		Example: `  # See what clearing browser caches would save
  gk clean browsers

  # See what uninstalling an iPhone app would free
  gk clean iphone-apps com.example.game`,
	}
	cmd.AddCommand(newCleanBrowsersCmd(global), newCleanIPhoneAppsCmd(global))
	return cmd
}

type cleanBrowsersOptions struct {
	browsers []string
	offline  bool
	apply    bool
}

func newCleanBrowsersCmd(global *globalOptions) *cobra.Command {
	opts := &cleanBrowsersOptions{}
	cmd := &cobra.Command{
		Use:   "browsers",
		Short: "Move browser caches to the Trash",
		Long: `Moves each browser's cache folders to the Trash: pages and code the
browser downloads again when needed. With --include-offline it also moves
offline data (service workers, Chrome's on-device AI model, crash reports).

Safety:
  - Dry run unless --apply: it lists what would move and the total.
  - Never touches site data: cookies, logins, history, Local Storage and
    IndexedDB. There is no flag to include them.
  - Refuses a browser that is running; quit it first.
  - Only touches folders inside the browsers' own data and cache folders,
    and never follows a symlink out of them.
  - Moves to the Trash, so you can put things back until you empty it.

See also: gk help safety, gk logs --cmd clean`,
		Example: `  # See what clearing Chrome and Firefox caches would save
  gk clean browsers --browser chrome,firefox

  # Actually move them to the Trash
  gk clean browsers --browser chrome,firefox --apply

  # Include offline data such as Chrome's on-device AI model
  gk clean browsers --browser chrome --include-offline --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCleanBrowsers(cmd, global, opts)
		},
	}
	cmd.Flags().StringSliceVar(&opts.browsers, "browser", nil, "browsers to clean: "+strings.Join(scan.BrowserKeys(), ", ")+" (default all)")
	cmd.RegisterFlagCompletionFunc("browser", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return installedBrowsers(home), cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().BoolVar(&opts.offline, "include-offline", false, "also move offline data, not just caches")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "move to the Trash; without it nothing changes")
	return cmd
}

// installedBrowsers lists the --browser names that have folders here.
func installedBrowsers(home string) []string {
	var keys []string
	for _, f := range scan.BrowserFolderList(home) {
		if slices.Contains(keys, f.Key) {
			continue
		}
		for _, p := range f.Paths {
			if _, err := os.Lstat(p); err == nil {
				keys = append(keys, f.Key)
				break
			}
		}
	}
	return keys
}

func runCleanBrowsers(cmd *cobra.Command, global *globalOptions, opts *cleanBrowsersOptions) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, false)
	if err != nil {
		return err
	}
	defer closeLog.Close()

	run := toolRunner
	if run == nil {
		run = scan.RunTool
	}
	log.Info("run started", logging.Flag("dry_run", !opts.apply))
	res, err := clean.Browsers{
		Home:    home,
		Keys:    opts.browsers,
		Offline: opts.offline,
		Apply:   opts.apply,
		Size:    size.New(folderTimeout, sizeWorkers),
		Run:     run,
		Log:     log,
	}.Clean(cmd.Context())
	if err != nil {
		log.Error("run failed", logging.ErrKind(err))
		return err
	}
	m := cleanMetrics(commandName(cmd), res, opts.apply)
	if err := history.SaveClean(history.Dir(home), m); err != nil {
		return fmt.Errorf("saving clean snapshot: %w", err)
	}
	log.Info("run finished", logging.Count("moved", m.Moved), logging.Bytes(m.MovedBytes),
		logging.Count("refused", m.Refused), logging.Count("failed", m.Failed), logging.Flag("dry_run", m.DryRun))
	printClean(cmd.OutOrStdout(), res, opts.apply)
	return nil
}

// cleanMetrics counts what a clean run moved, or would move in a dry run.
func cleanMetrics(command string, res clean.Result, apply bool) metrics.Clean {
	m := metrics.Clean{Time: time.Now(), Command: command, DryRun: !apply}
	for _, a := range res.Actions {
		switch a.Outcome {
		case clean.Moved, clean.WouldMove:
			m.Moved++
			m.MovedBytes += a.Bytes
		case clean.Refused:
			m.Refused++
		case clean.Failed:
			m.Failed++
		}
	}
	return m
}

func printClean(w io.Writer, res clean.Result, apply bool) {
	for _, a := range res.Actions {
		status := a.Outcome
		if a.Reason != "" {
			status += ": " + a.Reason
		}
		fmt.Fprintf(w, "  %10s  %-28s %s\n             %s\n", ui.Bytes(a.Bytes), a.Browser+" · "+a.Tier, status, a.Path)
	}
	for _, b := range res.Running {
		fmt.Fprintf(w, "%s is running, so it was skipped. Quit it and run the command again.\n", b)
	}
	switch {
	case len(res.Actions) == 0 && len(res.Running) == 0:
		fmt.Fprintln(w, "No browser caches found.")
	case apply:
		fmt.Fprintf(w, "Moved %s to the Trash.\n", ui.Bytes(res.Total(clean.Moved)))
	default:
		fmt.Fprintf(w, "Dry run: nothing was changed. Add --apply to move %s to the Trash.\n", ui.Bytes(res.Total(clean.WouldMove)))
	}
}
