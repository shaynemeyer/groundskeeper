// Package cli builds the groundskeeper command tree. Each command's help
// text lives next to its code.
package cli

import (
	"github.com/spf13/cobra"
)

const groupReport = "report"

type globalOptions struct {
	configPath string
	verbose    bool
}

// NewRoot returns the root command. With no subcommand it runs scan.
func NewRoot() *cobra.Command {
	global := &globalOptions{}
	scanOpts := &scanOptions{}

	root := &cobra.Command{
		Use:   "groundskeeper",
		Short: "Keep your Mac's disk free of clutter",
		Long: `Groundskeeper finds disk space you can reclaim (caches, AI models, stale
build artifacts and more) and writes an HTML report with a suggested command
for each item. Running it with no subcommand runs scan.

Safety: scanning is read-only. Nothing is deleted unless you run a clean
command yourself.`,
		Example: `  # Scan and write the report
  groundskeeper

  # Scan, then open the report in your browser
  gk --open`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, global, scanOpts)
		},
	}
	root.PersistentFlags().StringVar(&global.configPath, "config", "", "config file (default ~/.config/groundskeeper/config.json)")
	root.PersistentFlags().BoolVar(&global.verbose, "verbose", false, "also print debug log lines to stderr")
	addScanFlags(root, scanOpts)

	root.AddGroup(&cobra.Group{ID: groupReport, Title: "Report (read-only)"})
	root.AddCommand(newScanCmd(global))
	return root
}
