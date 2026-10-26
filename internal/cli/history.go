package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const dateFormat = "Mon 2 Jan 15:04"

func newHistoryCmd(global *globalOptions) *cobra.Command {
	runs := 10
	cmd := &cobra.Command{
		Use:     "history",
		GroupID: groupReport,
		Short:   "Show free-space trend and the fastest-growing items",
		Long: `Lists recent scans with free space and how it changed from the scan before,
then the items that grew most between the last two scans. Snapshots are
kept in ~/.local/state/groundskeeper for six months.

Safety: read-only.`,
		Example: `  # The last 10 scans
  gk history

  # The last 30 scans
  gk history --runs 30`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _, err := loadConfig(global)
			if err != nil {
				return err
			}
			scans, err := history.Scans(history.Dir(home))
			if err != nil {
				return err
			}
			printHistory(cmd.OutOrStdout(), scans, runs)
			return nil
		},
	}
	cmd.Flags().IntVar(&runs, "runs", runs, "how many recent scans to list")
	return cmd
}

func printHistory(w io.Writer, scans []history.Snapshot, runs int) {
	if len(scans) == 0 {
		fmt.Fprintln(w, "No scans yet. Run gk scan to take the first one.")
		return
	}
	first := max(0, len(scans)-runs)
	var rows [][]string
	for i := first; i < len(scans); i++ {
		s := scans[i]
		change := ""
		if i > 0 {
			change = ui.Delta(s.Free - scans[i-1].Free)
		}
		rows = append(rows, []string{s.Time.Format(dateFormat), ui.Bytes(s.Free), change, ui.Bytes(s.Total)})
	}
	plain := ui.Plain(w)
	fmt.Fprintln(w, ui.Table([]string{"Scan", "Free", "Change", "Disk"}, rows, []int{1, 2, 3}, plain))

	if len(scans) < 2 {
		return
	}
	last := scans[len(scans)-1]
	d := history.Compare(&scans[len(scans)-2], last)
	if len(d.Growing) == 0 {
		fmt.Fprintf(w, "\nNothing grew since %s.\n", d.Previous.Format(dateFormat))
		return
	}
	fmt.Fprintf(w, "\nGrowing fastest since %s:\n", d.Previous.Format(dateFormat))
	for _, c := range d.Growing {
		fmt.Fprintf(w, "  %10s  %s (%s)\n", ui.Grew(ui.Delta(c.Delta), plain), c.Label, c.Section)
	}
}
