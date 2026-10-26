package cli

import (
	"cmp"
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
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/report"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/size"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const (
	folderTimeout = 2 * time.Minute
	sizeWorkers   = 6
	topItems      = 5
)

type scanOptions struct {
	open bool
	dry  bool
}

func addScanFlags(cmd *cobra.Command, opts *scanOptions) {
	cmd.Flags().BoolVar(&opts.open, "open", false, "open the report after it's written")
	cmd.Flags().BoolVar(&opts.dry, "dry", false, "print a summary without writing any files")
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
never deletes anything. Suggested commands in the report are for you to run.`,
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

func runScan(cmd *cobra.Command, global *globalOptions, opts *scanOptions) error {
	ctx := cmd.Context()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := global.configPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath(home)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("reading config %s: %w", cfgPath, err)
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, opts.dry)
	if err != nil {
		return err
	}
	defer closeLog.Close()

	start := time.Now()
	log.Info("run started", logging.Flag("dry_run", opts.dry))
	disk, err := scan.DiskUsage(home)
	if err != nil {
		return err
	}
	rep := report.Report{Generated: start, Disk: disk}
	measurer := size.New(folderTimeout, sizeWorkers)
	minBytes := cfg.MinSizeMB * 1_000_000
	roots := expandAll(cfg.ProjectRoots, home)
	scanners := []scan.Scanner{
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
	}
	for _, s := range scanners {
		begin := time.Now()
		f, err := s.Scan(ctx, home)
		if err != nil {
			log.Error("scanner failed", logging.ID("scanner", s.Name()), logging.ErrKind(err))
			continue
		}
		log.Info("scanner done", logging.ID("scanner", s.Name()), logging.Duration(time.Since(begin)),
			logging.Count("items", len(f.Items)), logging.Count("skipped", len(f.Skipped)))
		for _, sk := range f.Skipped {
			log.Warn("path skipped", logging.Path(sk.Path), logging.ID("reason", sk.Reason))
		}
		rep.Sections = append(rep.Sections, report.Section{Name: s.Name(), Items: f.Items})
		rep.Skipped = append(rep.Skipped, f.Skipped...)
	}

	path := ""
	if !opts.dry {
		if path, err = report.Write(config.Expand(cfg.OutputDir, home), rep); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		log.Info("report written", logging.Path(path))
	}
	log.Info("run finished", logging.Duration(time.Since(start)), logging.Bytes(rep.Reclaimable()))
	printSummary(cmd.OutOrStdout(), rep, path)

	if opts.open && path != "" {
		return exec.CommandContext(ctx, "open", path).Run()
	}
	return nil
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
		return logging.Text(verbose, cmd.Name()), io.NopCloser(nil), nil
	}
	if dry {
		return logging.Discard(), io.NopCloser(nil), nil
	}
	path := filepath.Join(home, "Library", "Logs", "groundskeeper.log")
	return logging.Open(path, logging.Options{Level: cfg.LogLevel, Cmd: cmd.Name(), Verbose: verbose})
}

func printSummary(w io.Writer, rep report.Report, path string) {
	plain := ui.Plain(w)
	var b strings.Builder
	fmt.Fprintf(&b, "Free space   %s of %s\n", ui.Bytes(rep.Disk.Free), ui.Bytes(rep.Disk.Total))
	fmt.Fprintf(&b, "Reclaimable  %s (safe items)\n", ui.Bytes(rep.Reclaimable()))
	if len(rep.Skipped) > 0 {
		fmt.Fprintf(&b, "Skipped      %d paths (see report)\n", len(rep.Skipped))
	}
	if path != "" {
		fmt.Fprintf(&b, "Report       %s", path)
	} else {
		b.WriteString("Dry run: no files were written.")
	}
	fmt.Fprintln(w, ui.Box(b.String(), plain))

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
		fmt.Fprintf(w, "  %10s  %-6s  %s\n", ui.Bytes(it.Bytes), ui.Tag(it.Safety, plain), it.Label)
	}
}
