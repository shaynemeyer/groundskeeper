package cli

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// phoneUninstaller uninstalls from the connected iPhone. Tests replace it.
var phoneUninstaller = iphone.Uninstall

func newCleanIPhoneAppsCmd(global *globalOptions) *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "iphone-apps <bundleID>...",
		Short: "Uninstall iPhone apps by bundle ID",
		Long: `Uninstalls apps from the iPhone connected over USB. Bundle IDs are listed
in the report's iPhone section; run gk iphone first.

Safety:
  - Dry run unless --apply: it lists each app and what it would free.
  - Only accepts bundle IDs from the last gk iphone scan. Refuses Apple's
    apps and anything in iphoneKeep.
  - With --apply it asks before each app. Uninstalling deletes the app's
    data permanently, and the phone shows no prompt. To keep the data,
    offload the app instead in Settings › General › iPhone Storage.

See also: gk iphone, gk help safety, gk logs --cmd clean`,
		Example: `  # See what uninstalling an app would free
  gk clean iphone-apps com.example.game

  # Uninstall it, confirming first
  gk clean iphone-apps com.example.game --apply`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, ids []string) error {
			return runCleanIPhoneApps(cmd, global, ids, apply)
		},
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return snapshotBundleIDs(global), cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "uninstall after asking; without it nothing changes")
	return cmd
}

func runCleanIPhoneApps(cmd *cobra.Command, global *globalOptions, ids []string, apply bool) error {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return err
	}
	last, err := lastPhone(home)
	if err != nil {
		return err
	}
	log, closeLog, err := openLog(cmd, home, cfg, global, false)
	if err != nil {
		return err
	}
	defer closeLog.Close()

	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	log.Info("run started", logging.Flag("dry_run", !apply))
	actions := clean.IPhoneApps{
		IDs:       ids,
		Snapshot:  last.Phone,
		Keep:      cfg.IphoneKeep,
		Apply:     apply,
		Confirm:   func(a iphone.App) bool { return confirmUninstall(out, in, a) },
		Uninstall: phoneUninstaller,
		Log:       log,
	}.Clean()
	log.Info("run finished", logging.Count("apps", len(actions)), logging.Flag("dry_run", !apply))
	printUninstalls(out, actions, apply)
	return nil
}

// lastPhone returns the newest iPhone snapshot.
func lastPhone(home string) (history.PhoneSnapshot, error) {
	phones, err := history.Phones(history.PhoneDir(home))
	if err != nil || len(phones) == 0 {
		return history.PhoneSnapshot{}, fmt.Errorf("no iPhone scan found: run gk iphone first")
	}
	return phones[len(phones)-1], nil
}

// snapshotBundleIDs completes bundle IDs that clean iphone-apps would accept.
func snapshotBundleIDs(global *globalOptions) []string {
	home, cfg, err := loadConfig(global)
	if err != nil {
		return nil
	}
	last, err := lastPhone(home)
	if err != nil {
		return nil
	}
	var ids []string
	for _, a := range last.Phone.Apps {
		if !a.Apple() && !slices.Contains(cfg.IphoneKeep, a.BundleID) {
			ids = append(ids, a.BundleID+"\t"+a.Name)
		}
	}
	return ids
}

// confirmUninstall asks before one uninstall. Anything but y or yes is no.
func confirmUninstall(w io.Writer, in *bufio.Reader, a iphone.App) bool {
	fmt.Fprintf(w, "%s (%s): app %s, data %s.\n", a.Name, a.BundleID, ui.Bytes(a.AppBytes), ui.Bytes(a.DataBytes))
	fmt.Fprintln(w, "Its data will be deleted permanently. To keep it, offload the app in Settings › General › iPhone Storage instead.")
	fmt.Fprintf(w, "Uninstall %s? [y/N] ", a.Name)
	line, _ := in.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	fmt.Fprintln(w)
	return answer == "y" || answer == "yes"
}

func printUninstalls(w io.Writer, actions []clean.AppAction, apply bool) {
	var total int64
	var count int
	for _, a := range actions {
		status := a.Outcome
		if a.Reason != "" {
			status += ": " + a.Reason
		}
		if a.Outcome == clean.Uninstalled || a.Outcome == clean.WouldUninstall {
			total += a.App.Bytes()
			count++
		}
		fmt.Fprintf(w, "  %10s  %-28s %s\n             %s\n", ui.Bytes(a.App.Bytes()), a.App.Name, status, a.App.BundleID)
	}
	switch {
	case count == 0 && apply:
		fmt.Fprintln(w, "Nothing was uninstalled.")
	case count == 0:
		fmt.Fprintln(w, "Dry run: nothing would be uninstalled.")
	case apply:
		fmt.Fprintf(w, "Uninstalled apps freed about %s on the iPhone.\n", ui.Bytes(total))
	default:
		fmt.Fprintf(w, "Dry run: nothing was changed. Add --apply to uninstall and free about %s.\n", ui.Bytes(total))
	}
}
