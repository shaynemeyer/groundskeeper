package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/tui"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const groupReview = "review"

// reviewRunner runs the review screen. Tests replace it.
var reviewRunner = reviewInTerminal

func newReviewCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "review",
		GroupID: groupReview,
		Short:   "Review what the report found before cleaning it",
		Long: `Screens for deciding what a clean command may delete.

Safety: reviewing deletes nothing. It only saves your decisions.`,
		Example: `  # Step through duplicate photo groups
  gk review photos`,
	}
	cmd.AddCommand(newReviewPhotosCmd(global))
	return cmd
}

func newReviewPhotosCmd(global *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "photos",
		Short: "Step through duplicate photo groups",
		Long: `Shows the duplicate groups from the last gk iphone, one at a time, with the
best shot marked Keep and the rest marked Extra. Accept a group as it is,
change which photos are kept, or skip it. The extras of the groups you
accept are saved as the photos to delete.

Favorites, edited photos and photos in albums are always kept, and every
group keeps at least one photo.

Safety: deletes nothing. It saves your decisions to
~/.local/state/groundskeeper/photo-review.json, replacing the previous
review. Only gk clean photos --apply deletes, and only these photos.

See also: gk iphone, gk help safety`,
		Example: `  # Review the duplicates found by the last gk iphone
  gk review photos`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewPhotos(cmd, global)
		},
	}
}

func runReviewPhotos(cmd *cobra.Command, global *globalOptions) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	scan, err := history.LoadPhotoScan(history.PhotoScanPath(home))
	if errors.Is(err, history.ErrNoPhotoScan) {
		return fmt.Errorf("%w: run gk iphone first", err)
	}
	if err != nil {
		return fmt.Errorf("reading the photo scan: %w", err)
	}
	out := cmd.OutOrStdout()
	groups := scan.Photos.Duplicates.Groups
	if len(groups) == 0 {
		fmt.Fprintf(out, "The photo scan from %s found no duplicates.\n", scan.Time.Format(dateFormat))
		return nil
	}
	fmt.Fprintf(out, "Reviewing %d groups from the photo scan of %s.\n", len(groups), scan.Time.Format(dateFormat))
	r, err := reviewRunner(tui.NewReview(groups, scan.Photos.Assets), out)
	if errors.Is(err, errNoTerminal) {
		return fmt.Errorf("%w: run gk review photos in a terminal", err)
	}
	if err != nil {
		return err
	}
	if !r.Saved() {
		fmt.Fprintln(out, "Nothing saved.")
		return nil
	}
	sel, err := clean.NewSelection(time.Now(), scan.Photos.Source, scan.Photos.Assets, r.Delete())
	if err != nil {
		return err
	}
	path := clean.SelectionPath(home)
	if err := clean.SaveSelection(path, sel); err != nil {
		return fmt.Errorf("saving the review: %w", err)
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, false)
	if err != nil {
		return err
	}
	defer closeLog.Close()
	log.Info("photo review saved", logging.Path(path), logging.Count("photos", len(sel.Delete)), logging.Bytes(sel.Bytes()))
	fmt.Fprintf(out, "Saved %d photos to delete, %s. Nothing has been deleted.\n", len(sel.Delete), ui.Bytes(sel.Bytes()))
	return nil
}

// reviewInTerminal runs the review when stdin and out are both terminals.
func reviewInTerminal(r tui.Review, out io.Writer) (tui.Review, error) {
	f, ok := out.(*os.File)
	if !ok || !ui.IsTerminal(f) || !ui.IsTerminal(os.Stdin) {
		return r, errNoTerminal
	}
	m, err := tea.NewProgram(r, tea.WithOutput(out)).Run()
	if err != nil {
		return r, err
	}
	return m.(tui.Review), nil
}
