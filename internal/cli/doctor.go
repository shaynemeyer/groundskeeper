package cli

import (
	"fmt"
	"io"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

const groupSetup = "setup"

var statusStyles = map[string]lipgloss.Style{
	doctor.OK:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),
	doctor.Warn: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
	doctor.Fail: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
}

func newDoctorCmd(global *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		GroupID: groupSetup,
		Short:   "Check permissions, the schedule and where reports go",
		Long: `Checks that the binary and photos-helper are installed, whether protected
folders can be read (Full Disk Access), that the schedule is loaded and when
it runs next, when the last scheduled run happened, whether FileVault is on,
and that the report folder doesn't sync off this Mac. Each problem comes with
how to fix it.

Full Disk Access is checked for whatever runs doctor, usually your terminal.
The schedule needs it granted to ~/bin/groundskeeper separately.

Safety: read-only; doctor never changes settings.

See also: gk help privacy, gk help schedule`,
		Example: `  # Check everything
  gk doctor`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, cfg, err := loadConfig(global)
			if err != nil {
				return err
			}
			scans, _ := history.Scans(history.Dir(home))
			run := toolRunner
			if run == nil {
				run = scan.RunTool
			}
			checks := doctor.Run(cmd.Context(), doctor.Env{
				Home:      home,
				OutputDir: config.Expand(cfg.OutputDir, home),
				Run:       doctor.Runner(run),
				Now:       time.Now(),
				Scans:     scans,
			})
			if failed := printChecks(cmd.OutOrStdout(), checks); failed > 0 {
				return fmt.Errorf("%d check(s) failed; see the fixes above", failed)
			}
			return nil
		},
	}
}

func printChecks(w io.Writer, checks []doctor.Check) int {
	plain := ui.Plain(w)
	failed := 0
	for _, c := range checks {
		status := fmt.Sprintf("%-4s", c.Status)
		if !plain {
			status = statusStyles[c.Status].Render(status)
		}
		fmt.Fprintf(w, "%s  %-19s %s\n", status, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, "      %-19s Fix: %s\n", "", c.Fix)
		}
		if c.Status == doctor.Fail {
			failed++
		}
	}
	return failed
}
