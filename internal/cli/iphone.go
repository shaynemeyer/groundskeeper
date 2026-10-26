package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/report"
)

// phoneReader reads the connected iPhone. Tests replace it with a fixture.
var phoneReader = iphone.Read

func newIPhoneCmd(global *globalOptions) *cobra.Command {
	opts := &scanOptions{iphone: true}
	cmd := &cobra.Command{
		Use:     "iphone",
		GroupID: groupReport,
		Short:   "Scan a connected iPhone and add it to the report",
		Long: `Reads the iPhone connected over USB: free space, every app's size and data,
and the files apps share with the Mac. Then it reads the photos, from the
phone or from this Mac's Photos library, and finds duplicates, screenshots
older than screenshotDays and the largest videos; this takes a few
minutes. Last, it scans the Mac as gk scan does and writes the report with
an iPhone section.

The first run asks whether iCloud Photos is on, which decides where photos
are read. The iPhone must trust this Mac. If it asks, unlock it and tap
Trust.

Safety: read-only. Nothing on the phone or the Mac is changed or deleted.
Data sizes are what iOS reports over USB, which can differ from
Settings › General › iPhone Storage.

See also: gk scan, gk setup photos, gk help privacy`,
		Example: `  # Scan the iPhone and the Mac, and write the report
  gk iphone

  # Print a summary only, without writing the report, snapshot or log
  gk iphone --dry`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, global, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.open, "open", false, "open the report after it's written")
	cmd.Flags().BoolVar(&opts.dry, "dry", false, "print a summary without writing any files")
	return cmd
}

// readPhone reads and parses the iPhone, timed as a scanner for Run details.
func readPhone(log *logging.Logger, now time.Time) (*report.Phone, metrics.Scanner, error) {
	begin := time.Now()
	raw, err := phoneReader()
	m := metrics.Scanner{Name: "iPhone", DurationMS: time.Since(begin).Milliseconds()}
	if err != nil {
		log.Error("iphone read failed", logging.ID("error_kind", phoneKind(err)))
		return nil, m, phoneError(err)
	}
	p := iphone.Parse(raw)
	m.Items = len(p.Apps)
	m.Skipped = len(raw.Skipped)
	log.Info("iphone read", logging.Duration(time.Since(begin)), logging.Count("apps", len(p.Apps)),
		logging.Count("shared", len(p.Shared)), logging.Count("skipped", len(raw.Skipped)))
	return &report.Phone{Time: now, Phone: p}, m, nil
}

// savePhone saves the iPhone snapshot, prunes ones older than six months,
// and saves the photo scan for the review.
func savePhone(log *logging.Logger, home string, p report.Phone) error {
	dir := history.PhoneDir(home)
	if err := history.SavePhone(dir, history.PhoneSnapshot{Time: p.Time, Phone: p.Phone}); err != nil {
		return fmt.Errorf("saving iPhone snapshot: %w", err)
	}
	removed, err := history.Prune(dir, p.Time.AddDate(0, -historyMonths, 0))
	if err != nil {
		log.Warn("pruning iPhone snapshots failed", logging.Path(dir), logging.ErrKind(err))
	}
	log.Info("iphone snapshot saved", logging.Path(dir), logging.Count("pruned", removed))
	if p.Photos == nil {
		return nil
	}
	path := history.PhotoScanPath(home)
	if err := history.SavePhotoScan(path, history.PhotoScan{Time: p.Time, Photos: *p.Photos}); err != nil {
		return fmt.Errorf("saving photo scan: %w", err)
	}
	log.Info("photo scan saved", logging.Path(path))
	return nil
}

// phoneError says what to do about a failed read.
func phoneError(err error) error {
	switch {
	case errors.Is(err, iphone.ErrNoDevice):
		return fmt.Errorf("%w: connect it with a cable and try again", err)
	case errors.Is(err, iphone.ErrNotTrusted):
		return fmt.Errorf("%w: unlock it, tap Trust when it asks about this Mac, and try again", err)
	}
	return fmt.Errorf("%w. Unplug the iPhone, plug it in again and try again", err)
}

func phoneKind(err error) string {
	switch {
	case errors.Is(err, iphone.ErrNoDevice):
		return "no_device"
	case errors.Is(err, iphone.ErrNotTrusted):
		return "not_trusted"
	}
	return "read"
}

// markUnused flags apps whose data size hasn't changed in iphoneUnusedDays
// of earlier iPhone snapshots.
func markUnused(log *logging.Logger, home string, cfg config.Config, p *report.Phone) {
	dir := history.PhoneDir(home)
	past, err := history.Phones(dir)
	if err != nil {
		log.Warn("iphone history unreadable", logging.Path(dir), logging.ErrKind(err))
	}
	p.UnusedDays = cfg.IphoneUnusedDays
	p.Scans = len(past) + 1
	p.Unused, p.UnusedReady = history.Unused(past, p.Phone, p.Time, cfg.IphoneUnusedDays, cfg.IphoneKeep)
	log.Info("iphone unused", logging.Count("apps", len(p.Unused)), logging.Flag("ready", p.UnusedReady))
}
