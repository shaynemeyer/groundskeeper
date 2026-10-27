package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/tui"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// maxMacScanAge is how old the last scan may be for review mac, since its
// folders may have been reused since.
const maxMacScanAge = 7 * 24 * time.Hour

// macListRunner runs the Mac review screen. Tests replace it.
var macListRunner = macListInTerminal

func newReviewMacCmd(global *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "mac",
		Short: "Tick Mac items to move to the Trash",
		Long: `Lists the rows of the last scan that one move to the Trash fully cleans:
caches, build artifacts, old downloads, and Hugging Face and LM Studio
models. They're grouped by section, largest first, with each row's size,
safety tag and note. Nothing starts ticked. Tick the ones to move and save.

Rows that need their own tool, such as Ollama models, toolchains,
containers and apps, aren't listed; run their suggested command instead.
A scan older than 7 days is refused, since its folders may have changed.

Safety: moves nothing. It saves your choices to
~/.local/state/groundskeeper/mac-review.json, replacing the previous
review. Only gk clean mac --apply moves them, after checking each again.

See also: gk scan, gk clean mac, gk help safety`,
		Example: `  # Tick items from the last scan
  gk review mac`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewMac(cmd, global)
		},
	}
}

func runReviewMac(cmd *cobra.Command, global *globalOptions) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	scans, err := history.Scans(history.Dir(home))
	if err != nil {
		return fmt.Errorf("reading the last scan: %w", err)
	}
	if len(scans) == 0 {
		return errors.New("no scan has been saved: run gk scan first")
	}
	last := scans[len(scans)-1]
	if time.Since(last.Time) > maxMacScanAge {
		return fmt.Errorf("the last scan is from %s, more than 7 days ago, and its folders may have changed: run gk scan first",
			last.Time.Format(dateFormat))
	}
	out := cmd.OutOrStdout()
	list := tui.NewMacList(last.Items)
	if list.Len() == 0 {
		fmt.Fprintf(out, "Nothing in the scan of %s can be moved to the Trash.\n", last.Time.Format(dateFormat))
		return nil
	}
	list, err = macListRunner(list, out)
	if errors.Is(err, errNoTerminal) {
		return fmt.Errorf("%w: run gk review mac in a terminal", err)
	}
	if err != nil {
		return err
	}
	if !list.Saved() {
		fmt.Fprintln(out, "Nothing saved.")
		return nil
	}
	review, err := clean.NewMacReview(time.Now(), list.Ticked())
	if err != nil {
		return fmt.Errorf("%w; run gk scan and review again", err)
	}
	path := clean.MacReviewPath(home)
	if err := clean.SaveMacReview(path, review); err != nil {
		return fmt.Errorf("saving the review: %w", err)
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, false)
	if err != nil {
		return err
	}
	defer closeLog.Close()
	log.Info("mac review saved", logging.Path(path), logging.Count("items", len(review.Items)), logging.Bytes(review.Bytes()))
	fmt.Fprintf(out, "Saved %d items to move to the Trash, %s. Nothing has been moved; run gk clean mac --apply to move them.\n",
		len(review.Items), ui.Bytes(review.Bytes()))
	return nil
}

// macListInTerminal runs the list when stdin and out are both terminals.
func macListInTerminal(l tui.MacList, out io.Writer) (tui.MacList, error) {
	m, err := inTerminal(l, out)
	if err != nil {
		return l, err
	}
	return m.(tui.MacList), nil
}
