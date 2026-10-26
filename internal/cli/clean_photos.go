package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

func newCleanPhotosCmd(global *globalOptions) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "photos",
		Short: "Delete the photos you approved in gk review photos",
		Long: `Deletes the photos saved by the last gk review photos, from where they
were scanned: the iPhone connected over USB, or the Mac Photos library.

Safety:
  - Dry run unless --apply: it shows how many photos and how much space.
  - Only deletes photos saved by gk review photos; it takes no IDs.
  - A photo whose size or date changed since the scan is refused.
  - On the iPhone, deleting is permanent: photos skip Recently Deleted.
    --apply asks you to type the number of photos to confirm.
  - In the Mac library, --apply asks first, macOS asks again, and the
    photos stay in Recently Deleted for 30 days.

See also: gk review photos, gk help safety, gk logs --cmd clean`,
		Example: `  # See what the review approved
  gk clean photos

  # Delete those photos, confirming first
  gk clean photos --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCleanPhotos(cmd, global, apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "delete after confirming; without it nothing changes")
	return cmd
}

func runCleanPhotos(cmd *cobra.Command, global *globalOptions, apply bool) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	path := clean.SelectionPath(home)
	sel, err := clean.LoadSelection(path)
	if errors.Is(err, clean.ErrNoSelection) {
		return fmt.Errorf("%w: run gk review photos first", err)
	}
	if err != nil {
		return fmt.Errorf("reading the photo review: %w", err)
	}
	out := cmd.OutOrStdout()
	if len(sel.Delete) == 0 {
		fmt.Fprintln(out, "The saved review has no photos to delete.")
		return nil
	}
	fmt.Fprintf(out, "%d photos, %s, approved in the review of %s, will be deleted from %s.\n",
		len(sel.Delete), ui.Bytes(sel.Bytes()), sel.Time.Format(dateFormat), sourceName(sel.Source))
	if !apply {
		fmt.Fprintln(out, "Dry run: nothing was changed. Add --apply to delete them.")
		return nil
	}
	if !confirmPhotos(out, bufio.NewReader(cmd.InOrStdin()), sel) {
		fmt.Fprintln(out, "Nothing was deleted.")
		return nil
	}

	log, closeLog, err := openLog(cmd, home, cfg, global, false)
	if err != nil {
		return err
	}
	defer closeLog.Close()
	log.Info("run started", logging.Count("photos", len(sel.Delete)))
	h := iphone.Helper{Path: doctor.HelperPath(home), Source: sel.Source, Run: photoRunner}
	deleted, failed, err := h.Delete(cmd.Context(), sel.Requests())
	if err != nil {
		log.Info("run finished", logging.Count("deleted", 0))
		return errors.New("nothing was deleted: " + helperProblem(err))
	}
	left := sel.Without(deleted)
	if err := clean.SaveSelection(path, left); err != nil {
		return fmt.Errorf("updating the review: %w", err)
	}
	freed := sel.Bytes() - left.Bytes()
	log.Info("run finished", logging.Count("deleted", len(deleted)), logging.Count("failed", len(failed)), logging.Bytes(freed))
	printPhotoDeletes(out, len(deleted), freed, failed)
	return nil
}

// sourceName says where photos from source live.
func sourceName(source string) string {
	if source == iphone.SourceMacLibrary {
		return "the Mac Photos library"
	}
	return "the iPhone"
}

// confirmPhotos asks before deleting. On the iPhone, where deletes are
// permanent, the user must type the number of photos; in the Mac library
// y or yes is enough, since macOS asks again.
func confirmPhotos(w io.Writer, in *bufio.Reader, sel clean.Selection) bool {
	n := len(sel.Delete)
	if sel.Source == iphone.SourceMacLibrary {
		fmt.Fprintln(w, "macOS will ask you to confirm too, and they stay in Recently Deleted for 30 days.")
		fmt.Fprint(w, "Delete them? [y/N] ")
	} else {
		fmt.Fprintln(w, "This is permanent: they don't go to the iPhone's Recently Deleted.")
		fmt.Fprintf(w, "Type %d to delete them: ", n)
	}
	line, _ := in.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	fmt.Fprintln(w)
	if sel.Source == iphone.SourceMacLibrary {
		return answer == "y" || answer == "yes"
	}
	return answer == strconv.Itoa(n)
}

func printPhotoDeletes(w io.Writer, deleted int, freed int64, failed []iphone.Failure) {
	fmt.Fprintf(w, "Deleted %d photos, freeing about %s.\n", deleted, ui.Bytes(freed))
	if len(failed) == 0 {
		return
	}
	fmt.Fprintf(w, "%d photos weren't deleted and stay in the saved review:\n", len(failed))
	for _, f := range failed {
		fmt.Fprintf(w, "  %s: %s\n", f.ID, failureReason(f.Reason))
	}
}

// failureReason explains a photos-helper delete failure.
func failureReason(reason string) string {
	switch reason {
	case "not-found":
		return "no longer there"
	case "mismatch":
		return "changed since the scan, so it was refused; run gk iphone and review again"
	case "delete":
		return "Photos refused to delete it"
	}
	return reason
}
