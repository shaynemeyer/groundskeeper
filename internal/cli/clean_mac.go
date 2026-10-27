package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

func newCleanMacCmd(global *globalOptions) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "mac",
		Short: "Move the Mac items you ticked in gk review mac to the Trash",
		Long: `Moves the items saved by the last gk review mac to the Trash: caches,
build artifacts, old downloads and AI models from the last scan.

Safety:
  - Dry run unless --apply: it shows how many items and how much space.
  - Only moves items saved by gk review mac; it takes no paths.
  - Each item is checked again before it moves: it must still be the kind
    of thing its section cleans (a cache, a build artifact in a project
    folder, a download, a Hugging Face or LM Studio model), inside your
    home folder, reached without a symlink, and still a file or a folder
    as it was. Anything else is refused and left where it is.
  - Moves to the Trash, so you can put things back until you empty it.
    The space comes back when the Trash is emptied.

See also: gk review mac, gk help safety, gk logs --cmd clean`,
		Example: `  # See what the review ticked
  gk clean mac

  # Move those items to the Trash, confirming first
  gk clean mac --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCleanMac(cmd, global, apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "move to the Trash after confirming; without it nothing changes")
	return cmd
}

func runCleanMac(cmd *cobra.Command, global *globalOptions, apply bool) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	path := clean.MacReviewPath(home)
	review, err := clean.LoadMacReview(path)
	if errors.Is(err, clean.ErrNoMacReview) {
		return fmt.Errorf("%w: run gk review mac first", err)
	}
	if err != nil {
		return fmt.Errorf("reading the Mac review: %w", err)
	}
	out := cmd.OutOrStdout()
	if len(review.Items) == 0 {
		fmt.Fprintln(out, "The saved review has no items to move.")
		return nil
	}
	fmt.Fprintf(out, "%d items, %s, ticked in the review of %s, will be moved to the Trash.\n",
		len(review.Items), ui.Bytes(review.Bytes()), review.Time.Format(dateFormat))
	if !apply {
		fmt.Fprintln(out, "Dry run: nothing was changed. Add --apply to move them to the Trash.")
		return nil
	}
	if !confirmTrash(out, bufio.NewReader(cmd.InOrStdin())) {
		fmt.Fprintln(out, "Nothing was moved.")
		return nil
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
	log.Info("run started", logging.Count("items", len(review.Items)))
	rules := clean.MacRules{Home: home, Roots: expandAll(cfg.ProjectRoots, home)}
	actions := clean.Mac{Rules: rules, Run: run, Log: log}.Clean(cmd.Context(), review)

	m := macMetrics(commandName(cmd), actions)
	if err := clean.SaveMacReview(path, review.Without(movedPaths(actions))); err != nil {
		return fmt.Errorf("updating the review: %w", err)
	}
	if err := history.SaveClean(history.Dir(home), m); err != nil {
		return fmt.Errorf("saving clean snapshot: %w", err)
	}
	log.Info("run finished", logging.Count("moved", m.Moved), logging.Bytes(m.MovedBytes),
		logging.Count("refused", m.Refused), logging.Count("failed", m.Failed))
	printMacMoves(out, m, actions)
	return nil
}

// confirmTrash asks before moving anything; only y or yes goes ahead.
func confirmTrash(w io.Writer, in *bufio.Reader) bool {
	fmt.Fprint(w, "Move them to the Trash? [y/N] ")
	line, _ := in.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	fmt.Fprintln(w)
	return answer == "y" || answer == "yes"
}

func movedPaths(actions []clean.MacAction) []string {
	var paths []string
	for _, a := range actions {
		if a.Outcome == clean.Moved {
			paths = append(paths, a.Item.Trash)
		}
	}
	return paths
}

// macMetrics counts what a clean mac run moved, refused and failed on.
func macMetrics(command string, actions []clean.MacAction) metrics.Clean {
	m := metrics.Clean{Time: time.Now(), Command: command}
	for _, a := range actions {
		switch a.Outcome {
		case clean.Moved:
			m.Moved++
			m.MovedBytes += a.Item.Bytes
		case clean.Refused:
			m.Refused++
		case clean.Failed:
			m.Failed++
		}
	}
	return m
}

func printMacMoves(w io.Writer, m metrics.Clean, actions []clean.MacAction) {
	fmt.Fprintf(w, "Moved %d items, about %s, to the Trash. The space comes back when you empty the Trash.\n",
		m.Moved, ui.Bytes(m.MovedBytes))
	if m.Refused+m.Failed == 0 {
		return
	}
	fmt.Fprintf(w, "%d items weren't moved and stay in the saved review:\n", m.Refused+m.Failed)
	for _, a := range actions {
		if a.Outcome != clean.Moved {
			fmt.Fprintf(w, "  %s: %s\n    %s\n", a.Item.Label, a.Reason, a.Item.Trash)
		}
	}
}
