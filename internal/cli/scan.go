package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/notify"
	"github.com/shaynemeyer/groundskeeper/internal/report"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const (
	sizeWorkers   = 6
	topItems      = 5
	historyMonths = 6
)

// Tests replace these so scans stay inside a fake home directory.
var (
	appDirs    = func(home string) []string { return nil } // nil: /Applications and ~/Applications
	toolRunner scan.Runner                                 // nil: the real podman, mdls, plutil, ps and brew
	// folderTimeout is how long the size engine spends on one folder.
	folderTimeout = 2 * time.Minute
)

type scanOptions struct {
	open   bool
	dry    bool
	notify bool
	iphone bool // read the connected iPhone first, and fail if it can't be read
}

func addScanFlags(cmd *cobra.Command, opts *scanOptions) {
	cmd.Flags().BoolVar(&opts.open, "open", false, "open the report after it's written")
	cmd.Flags().BoolVar(&opts.dry, "dry", false, "print a summary without writing any files")
	cmd.Flags().BoolVar(&opts.notify, "notify", false, "post a macOS notification when done (the schedule uses this)")
}

func newScanCmd(global *globalOptions) *cobra.Command {
	opts := &scanOptions{}
	cmd := &cobra.Command{
		Use:     "scan",
		GroupID: groupReport,
		Short:   "Scan the Mac and write the HTML report (default command)",
		Long: `Measures caches and other reclaimable space, then writes the report to
~/Reports/groundskeeper/YYYY-MM-DD.html and latest.html, and prints a summary.

Safety: read-only. Scan only measures sizes, never reads file contents, and
never deletes anything. Suggested commands in the report are for you to run.

See also: gk help safety, gk history, gk stats`,
		Example: `  # Scan and write the report
  gk scan

  # Print a summary only, without writing the report or log
  gk scan --dry

  # Scan and open the report in your browser
  gk scan --open`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, global, opts)
		},
	}
	addScanFlags(cmd, opts)
	return cmd
}

// configPath returns the config file the --config flag names, or the default.
func configPath(global *globalOptions, home string) string {
	if global.configPath != "" {
		return global.configPath
	}
	return config.DefaultPath(home)
}

// loadConfig returns the home folder and the config the --config flag names.
func loadConfig(global *globalOptions) (string, config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", config.Config{}, err
	}
	path := configPath(global, home)
	cfg, err := config.Load(path)
	if err != nil {
		return "", cfg, fmt.Errorf("reading config %s: %w", path, err)
	}
	return home, cfg, nil
}

func runScan(cmd *cobra.Command, global *globalOptions, opts *scanOptions) error {
	ctx := cmd.Context()
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, opts.dry)
	if err != nil {
		return err
	}
	defer closeLog.Close()
	if where := doctor.SyncedFolder(config.Expand(cfg.OutputDir, home), home); where != "" {
		log.Warn("report folder syncs off this Mac", logging.Path(cfg.OutputDir))
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: the report folder is in %s, so reports may leave this Mac. See gk help privacy.\n", where)
	}

	var source, photosNote string
	if opts.iphone {
		if source, photosNote, err = photosSource(cmd, global, home, cfg, opts.dry); err != nil {
			return err
		}
	}

	start := time.Now()
	log.Info("run started", logging.Flag("dry_run", opts.dry))
	spin := startSpinner(cmd, global)
	defer spin.Stop()
	var phone *report.Phone
	var phoneStats []metrics.Scanner
	if opts.iphone {
		spin.Set("Reading iPhone")
		var stat metrics.Scanner
		if phone, stat, err = readPhone(log, start); err != nil {
			return err
		}
		phoneStats = append(phoneStats, stat)
		markUnused(log, home, cfg, phone)
		phone.PhotosNote = photosNote
		phone.ScreenshotDays = cfg.ScreenshotDays
		if source != "" {
			spin.Set("Reading photos (this takes a few minutes)")
			phone.Photos, stat, phone.PhotosNote = scanPhotos(ctx, log, home, cfg, source, start)
			phoneStats = append(phoneStats, stat)
		}
	}
	disk, err := scan.DiskUsage(home)
	if err != nil {
		return err
	}
	measurer := size.New(folderTimeout, sizeWorkers)
	rep := report.Report{Generated: start, Disk: disk}
	rep.Sections, rep.Skipped, rep.Run.Scanners = runScanners(ctx, log, newScanners(home, cfg, measurer), home, spin)
	// Once ctx is cancelled (Ctrl-C), every folder not yet measured is
	// recorded as a timeout, so the result is too partial to save.
	if ctx.Err() != nil {
		log.Warn("run interrupted", logging.ErrKind(ctx.Err()))
		return errors.New("scan interrupted; no report or snapshot saved")
	}
	if phone != nil {
		rep.IPhone = phone
		rep.Run.Scanners = append(phoneStats, rep.Run.Scanners...)
	}
	rep.Run.Start = start
	rep.Run.DurationMS = time.Since(start).Milliseconds()
	rep.Run.Command = commandName(cmd)
	rep.Run.Interactive = ui.IsTerminal(os.Stdin)
	rep.Run.Size = measurer.Stats()
	if err := addHistory(log, home, &rep, opts.dry); err != nil {
		return err
	}
	if phone != nil && !opts.dry {
		if err := savePhone(log, home, *phone); err != nil {
			return err
		}
	}

	spin.Set("Writing report")
	path := ""
	if !opts.dry {
		if path, err = report.Write(config.Expand(cfg.OutputDir, home), rep); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		log.Info("report written", logging.Path(path))
	}
	log.Info("run finished", logging.Duration(time.Since(start)), logging.Bytes(rep.Reclaimable()))
	spin.Stop()
	printSummary(cmd.OutOrStdout(), rep, path)
	if opts.notify && !opts.dry {
		sendNotification(ctx, log, rep)
	}

	if opts.open && path != "" {
		return exec.CommandContext(ctx, "open", path).Run()
	}
	return nil
}

func newScanners(home string, cfg config.Config, measurer *size.Measurer) []scan.Scanner {
	minBytes := cfg.MinSizeMB * 1_000_000
	roots := expandAll(cfg.ProjectRoots, home)
	return []scan.Scanner{
		scan.Caches{Size: measurer, MinBytes: minBytes},
		scan.Models{Size: measurer, ProjectRoots: roots, MinBytes: minBytes},
		scan.Artifacts{
			Size:      measurer,
			Roots:     roots,
			Exclude:   expandAll(cfg.ExcludePaths, home),
			StaleDays: cfg.StaleDays,
			MinBytes:  minBytes,
		},
		scan.Toolchains{Size: measurer, MinBytes: minBytes},
		scan.Containers{Size: measurer, Run: toolRunner, MinBytes: minBytes},
		scan.Downloads{Size: measurer, StaleDays: cfg.StaleDays, MinBytes: minBytes},
		scan.Browsers{Size: measurer, MinBytes: minBytes},
		scan.Apps{
			Size:       measurer,
			Run:        toolRunner,
			Dirs:       appDirs(home),
			UnusedDays: cfg.AppUnusedDays,
			Exclude:    cfg.AppExclude,
			MinBytes:   minBytes,
		},
	}
}

// runScanners runs each scanner in turn, logging and timing it. A failed
// scanner is recorded and the rest still run.
func runScanners(ctx context.Context, log *logging.Logger, scanners []scan.Scanner, home string, spin *ui.Spinner) ([]report.Section, []size.Skip, []metrics.Scanner) {
	var sections []report.Section
	var skipped []size.Skip
	var stats []metrics.Scanner
	for _, s := range scanners {
		spin.Set("Scanning " + s.Name())
		begin := time.Now()
		f, err := s.Scan(ctx, home)
		m := metrics.Scanner{Name: s.Name(), DurationMS: time.Since(begin).Milliseconds(), Items: len(f.Items), Skipped: len(f.Skipped)}
		if err != nil {
			m.Error = logging.KindOf(err)
			log.Error("scanner failed", logging.ID("scanner", s.Name()), logging.ErrKind(err))
			stats = append(stats, m)
			continue
		}
		for _, it := range f.Items {
			m.Bytes += it.Bytes
		}
		for _, sk := range f.Skipped {
			if sk.Reason == "timeout" {
				m.Timeouts++
			}
			log.Warn("path skipped", logging.Path(sk.Path), logging.ID("reason", sk.Reason))
		}
		log.Info("scanner done", logging.ID("scanner", s.Name()), logging.Duration(time.Since(begin)),
			logging.Count("items", m.Items), logging.Count("skipped", m.Skipped))
		stats = append(stats, m)
		sections = append(sections, report.Section{Name: s.Name(), Items: f.Items})
		skipped = append(skipped, f.Skipped...)
	}
	return sections, skipped, stats
}

// addHistory compares rep with the previous snapshot and, unless dry,
// saves rep as a new snapshot and prunes ones older than six months.
func addHistory(log *logging.Logger, home string, rep *report.Report, dry bool) error {
	dir := history.Dir(home)
	scans, err := history.Scans(dir)
	if err != nil {
		log.Warn("history unreadable", logging.Path(dir), logging.ErrKind(err))
	}
	cur := snapshotOf(*rep)
	var prev *history.Snapshot
	var since time.Time
	if len(scans) > 0 {
		prev = &scans[len(scans)-1]
		since = prev.Time
	}
	rep.Deltas = history.Compare(prev, cur)
	cleans, _ := history.Cleans(dir, since)
	for _, c := range cleans {
		if !c.DryRun {
			rep.Freed += c.MovedBytes
		}
	}
	if dry {
		return nil
	}
	if err := history.SaveScan(dir, cur); err != nil {
		return fmt.Errorf("saving snapshot: %w", err)
	}
	removed, err := history.Prune(dir, rep.Generated.AddDate(0, -historyMonths, 0))
	if err != nil {
		log.Warn("pruning snapshots failed", logging.Path(dir), logging.ErrKind(err))
	}
	log.Info("snapshot saved", logging.Path(dir), logging.Count("pruned", removed))
	return nil
}

func snapshotOf(rep report.Report) history.Snapshot {
	s := history.Snapshot{Time: rep.Generated, Free: rep.Disk.Free, Total: rep.Disk.Total, Run: rep.Run, Skipped: []string{}}
	for _, sk := range rep.Skipped {
		s.Skipped = append(s.Skipped, sk.Path)
	}
	for _, sec := range rep.Sections {
		for _, it := range sec.Items {
			s.Items = append(s.Items, history.Item{
				Section: it.Section, Label: it.Label, Path: it.Path, Bytes: it.Bytes,
				Safety: it.Safety, Note: it.Note, Trash: it.Trash,
			})
		}
	}
	return s
}

// commandName is the command path without the program name, such as
// "clean browsers", or "scan" when the root runs a scan.
func commandName(cmd *cobra.Command) string {
	if !cmd.HasParent() {
		return "scan"
	}
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}

// sendNotification posts the end-of-scan notification. A failure is
// logged, not returned, because the report was still written.
func sendNotification(ctx context.Context, log *logging.Logger, rep report.Report) {
	run := toolRunner
	if run == nil {
		run = scan.RunTool
	}
	msg := notify.Message(notify.Summary{
		Free:        rep.Disk.Free,
		Reclaimable: rep.Reclaimable(),
		Problems:    rep.Run.Problems(),
		NewSkipped:  rep.Deltas.NewSkip,
	})
	if err := notify.Send(ctx, notify.Runner(run), msg); err != nil {
		log.Warn("notification failed", logging.ErrKind(err))
		return
	}
	log.Info("notification sent", logging.Count("failed_scanners", rep.Run.Errors()), logging.Count("new_skipped", rep.Deltas.NewSkip))
}

// changeNote is " (+2.1 GB)" for an item that changed since the last run,
// red when it grew.
func changeNote(rep report.Report, it scan.Item, plain bool) string {
	change := rep.Change(it)
	if change == "" {
		return ""
	}
	if rep.Grew(it) {
		change = ui.Grew(change, plain)
	}
	return "  (" + change + ")"
}

func expandAll(paths []string, home string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = config.Expand(p, home)
	}
	return out
}

func openLog(cmd *cobra.Command, home string, cfg config.Config, global *globalOptions, dry bool) (*logging.Logger, io.Closer, error) {
	var verbose io.Writer
	if global.verbose {
		verbose = cmd.ErrOrStderr()
	}
	if dry && verbose != nil {
		return logging.Text(verbose, commandName(cmd)), io.NopCloser(nil), nil
	}
	if dry {
		return logging.Discard(), io.NopCloser(nil), nil
	}
	path := filepath.Join(home, "Library", "Logs", "groundskeeper.log")
	return logging.Open(path, logging.Options{Level: cfg.LogLevel, Cmd: commandName(cmd), Verbose: verbose})
}

func printSummary(w io.Writer, rep report.Report, path string) {
	plain := ui.Plain(w)
	var b strings.Builder
	fmt.Fprintf(&b, "Free space   %s of %s\n", ui.Bytes(rep.Disk.Free), ui.Bytes(rep.Disk.Total))
	fmt.Fprintf(&b, "Reclaimable  %s (safe items)\n", ui.Bytes(rep.Reclaimable()))
	if p := rep.IPhone; p != nil {
		fmt.Fprintf(&b, "iPhone       %s free of %s\n", ui.Bytes(p.Phone.FreeBytes), ui.Bytes(p.Phone.TotalBytes))
		if p.Photos != nil {
			fmt.Fprintf(&b, "Photos       %d duplicate groups, %d old screenshots\n", len(p.Photos.Duplicates.Groups), len(p.Photos.OldScreenshots))
		}
	}
	if rep.Deltas.HasPrevious() {
		fmt.Fprintf(&b, "Since %s  %s free space\n", rep.Deltas.Previous.Format(dateFormat), ui.Delta(rep.Deltas.FreeDelta))
	}
	if len(rep.Skipped) > 0 {
		fmt.Fprintf(&b, "Skipped      %d paths (see report)\n", len(rep.Skipped))
	}
	if path != "" {
		fmt.Fprintf(&b, "Report       %s", path)
	} else {
		b.WriteString("Dry run: no files were written.")
	}
	fmt.Fprintln(w, ui.Box(b.String(), plain))
	if p := rep.IPhone; p != nil && p.PhotosNote != "" {
		fmt.Fprintln(w, p.PhotosNote)
	}

	var items []scan.Item
	for _, s := range rep.Sections {
		items = append(items, s.Items...)
	}
	if len(items) == 0 {
		return
	}
	slices.SortFunc(items, func(a, b scan.Item) int { return cmp.Compare(b.Bytes, a.Bytes) })
	fmt.Fprintln(w, "\nLargest items:")
	for _, it := range items[:min(topItems, len(items))] {
		fmt.Fprintf(w, "  %10s  %-6s  %s%s\n", ui.Bytes(it.Bytes), ui.Tag(it.Safety, plain), it.Label, changeNote(rep, it, plain))
	}
}

// startSpinner shows progress on stderr. It stays silent with --verbose,
// whose log lines go to stderr too, and when stdout or stderr is plain, so
// piping the summary into a pager doesn't draw over it.
func startSpinner(cmd *cobra.Command, global *globalOptions) *ui.Spinner {
	if global.verbose || ui.Plain(cmd.OutOrStdout()) {
		return &ui.Spinner{}
	}
	return ui.StartSpinner(cmd.ErrOrStderr())
}
