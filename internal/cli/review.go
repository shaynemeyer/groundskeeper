package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
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
best shot marked Keep and the rest marked Extra. In Ghostty the photos are
shown in the screen; in any terminal, press space to see them in Quick
Look. They're fetched from the phone or the Photos library into a private
temporary folder that's removed when the review ends. Accept a group as
it is, change which photos are kept, or skip it. The extras of the groups you
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
	dir, err := os.MkdirTemp("", "groundskeeper-review-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	inline := os.Getenv("TERM_PROGRAM") == "ghostty"
	viewer := photoViewer(cmd.Context(), home, scan.Photos.Source, dir, inline)
	r, err := reviewRunner(tui.NewReview(groups, scan.Photos.Assets, viewer), out)
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

// photoViewer fetches thumbnails through photos-helper into dir, which
// os.MkdirTemp made 0700, and previews them with Quick Look. Inline shows
// them in the screen too, which only Ghostty has been tested to do.
func photoViewer(ctx context.Context, home, source, dir string, inline bool) tui.Viewer {
	h := iphone.Helper{Path: doctor.HelperPath(home), Source: source, Run: photoRunner}
	return tui.Viewer{
		Thumbnails: func(ids []string) (map[string]string, error) {
			thumbs, _, err := h.Thumbnails(ctx, ids, dir)
			if err != nil {
				return nil, errors.New(thumbnailProblem(err))
			}
			paths := map[string]string{}
			for _, t := range thumbs {
				paths[t.ID] = t.Path
			}
			return paths, nil
		},
		Preview: func(paths []string) *exec.Cmd {
			return exec.CommandContext(ctx, "qlmanage", append([]string{"-p"}, paths...)...)
		},
		Inline: inline,
	}
}

// thumbnailProblem says briefly why thumbnails couldn't be fetched.
func thumbnailProblem(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "photos-helper isn't installed. Run make install."
	case errors.Is(err, iphone.ErrNoDevice):
		return "no iPhone answered. Connect and unlock it."
	case errors.Is(err, iphone.ErrLocked):
		return "the iPhone is locked. Unlock it."
	case errors.Is(err, iphone.ErrDenied):
		return "this terminal app doesn't have Photos access."
	case errors.Is(err, iphone.ErrTimeout):
		return "the iPhone didn't answer in time."
	}
	return "photos-helper failed."
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
	r = m.(tui.Review)
	fmt.Fprint(out, r.Cleanup())
	return r, nil
}
