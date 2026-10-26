package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

type statsOptions struct {
	runs int
	json bool
}

func newStatsCmd(global *globalOptions) *cobra.Command {
	opts := &statsOptions{runs: 1}
	cmd := &cobra.Command{
		Use:     "stats",
		GroupID: groupReport,
		Short:   "Show scan timings, skipped paths and cleanup totals",
		Long: `Shows how the last scan went: each scanner's time, items, size, skipped paths
and status, how much the size engine visited, and what clean commands did
since. With --runs, compares scanner times across recent scans, which shows
a scanner that keeps getting slower.

Safety: read-only. The numbers come from the snapshots in
~/.local/state/groundskeeper and never leave this Mac.`,
		Example: `  # How the last scan went
  gk stats

  # Scanner times over the last 5 scans
  gk stats --runs 5

  # Raw metrics as JSON
  gk stats --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _, err := loadConfig(global)
			if err != nil {
				return err
			}
			dir := history.Dir(home)
			scans, err := history.Scans(dir)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(scans) == 0 {
				fmt.Fprintln(w, "No scans yet. Run gk scan to take the first one.")
				return nil
			}
			recent := scans[max(0, len(scans)-opts.runs):]
			switch {
			case opts.json:
				return printStatsJSON(w, recent)
			case len(recent) > 1:
				printScannerTrend(w, recent)
				return nil
			}
			cleans, _ := history.Cleans(dir, recent[0].Time)
			printLastRun(w, recent[0].Run, cleans)
			return nil
		},
	}
	cmd.Flags().IntVar(&opts.runs, "runs", opts.runs, "how many recent scans to include")
	cmd.Flags().BoolVar(&opts.json, "json", false, "print the raw metrics as JSON")
	return cmd
}

func printStatsJSON(w io.Writer, scans []history.Snapshot) error {
	runs := make([]metrics.Run, len(scans))
	for i, s := range scans {
		runs[i] = s.Run
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(runs)
}

func printLastRun(w io.Writer, r metrics.Run, cleans []metrics.Clean) {
	plain := ui.Plain(w)
	how := "scheduled or piped"
	if r.Interactive {
		how = "from a terminal"
	}
	fmt.Fprintf(w, "Last scan %s, %s, took %.1f s\n\n", r.Start.Format(dateFormat), how, float64(r.DurationMS)/1000)
	var rows [][]string
	for _, s := range r.Scanners {
		rows = append(rows, []string{s.Name, fmt.Sprintf("%.1f s", float64(s.DurationMS)/1000),
			fmt.Sprint(s.Items), ui.Bytes(s.Bytes), fmt.Sprint(s.Skipped), s.Status()})
	}
	fmt.Fprintln(w, ui.Table([]string{"Scanner", "Time", "Items", "Size", "Skipped", "Status"}, rows, []int{1, 2, 3, 4}, plain))
	fmt.Fprintf(w, "\nVisited %d files and %d folders; %d hardlinks counted once.\n", r.Size.Files, r.Size.Dirs, r.Size.HardlinksDeduped)

	var moved int64
	applied := 0
	for _, c := range cleans {
		if !c.DryRun {
			applied++
			moved += c.MovedBytes
		}
	}
	fmt.Fprintf(w, "Clean runs since: %d applied, %s moved to the Trash.\n", applied, ui.Bytes(moved))
}

// printScannerTrend shows each scanner's time across scans, oldest first.
func printScannerTrend(w io.Writer, scans []history.Snapshot) {
	headers := []string{"Scanner"}
	right := []int{}
	for i, s := range scans {
		headers = append(headers, s.Time.Format("2 Jan 15:04"))
		right = append(right, i+1)
	}
	var names []string
	for _, s := range scans {
		for _, sc := range s.Run.Scanners {
			if !slices.Contains(names, sc.Name) {
				names = append(names, sc.Name)
			}
		}
	}
	var rows [][]string
	for _, name := range names {
		row := []string{name}
		for _, s := range scans {
			cell := "-"
			for _, sc := range s.Run.Scanners {
				if sc.Name == name {
					cell = fmt.Sprintf("%.1f s", float64(sc.DurationMS)/1000)
				}
			}
			row = append(row, cell)
		}
		rows = append(rows, row)
	}
	fmt.Fprintln(w, ui.Table(headers, rows, right, ui.Plain(w)))
}
