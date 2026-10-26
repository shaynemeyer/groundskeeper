package cli

import "github.com/spf13/cobra"

const groupTopics = "topics"

// Help topics are commands without a Run, which Cobra lists under
// "Additional help topics" and shows with gk help <topic>.
func newTopics() []*cobra.Command {
	return []*cobra.Command{
		{
			Use:     "safety",
			GroupID: groupTopics,
			Short:   "What groundskeeper never deletes, and how dry runs work",
			Long: `Scanning is read-only. The report and the schedule only measure and suggest
commands; they never delete anything.

Only clean commands remove things, and only when you run them:
  - Every clean command is a dry run unless you add --apply.
  - clean browsers only moves cache folders (and offline data with
    --include-offline) to the Trash. It never touches cookies, logins,
    history or other site data, refuses a browser that is running, and never
    follows a symlink outside the browser's own folders.
  - Anything moved goes to the Trash, so you can put it back until you
    empty it.

The only files groundskeeper deletes on its own are its own snapshots in
~/.local/state/groundskeeper, after six months.

Every clean action is logged: gk logs --cmd clean`,
		},
		{
			Use:     "schedule",
			GroupID: groupTopics,
			Short:   "How the launchd job works and how to change it",
			Long: `make install loads a LaunchAgent, ~/Library/LaunchAgents/com.lot86.groundskeeper.plist,
that runs "groundskeeper scan --notify" on Mondays and Thursdays at 09:00.

  - If the Mac is asleep then, launchd runs the job when it wakes.
  - launchd applies its default light limits on CPU and disk use.
  - It only ever runs the read-only scan, never a clean command.
  - Its own output goes to ~/Library/Logs/groundskeeper.launchd.log; the
    scan's log is ~/Library/Logs/groundskeeper.log as usual.

To change the days or time, edit launchd/com.lot86.groundskeeper.plist in the
repo (StartCalendarInterval; Weekday 0 or 7 is Sunday) and run make install
again. make uninstall removes the schedule.

Check it with gk doctor, or run it now with:
  launchctl kickstart gui/$(id -u)/com.lot86.groundskeeper`,
		},
		{
			Use:     "privacy",
			GroupID: groupTopics,
			Short:   "Full Disk Access, FileVault, backups and what stays on this Mac",
			Long: `Nothing leaves this Mac: no network requests, no telemetry. Scans measure
sizes without reading file contents. Reports, snapshots and logs hold paths,
sizes, dates and IDs only, and are readable only by you.

Full Disk Access
  macOS protects Downloads, Mail, Safari and some browser and app data.
  Without Full Disk Access those are listed under Skipped paths. Grant it in
  System Settings > Privacy & Security > Full Disk Access: to your terminal
  for runs you start, and to ~/bin/groundskeeper for the schedule.

FileVault
  Reports, snapshots and logs aren't encrypted by groundskeeper; FileVault
  encrypts the whole disk. gk doctor warns if it's off.

Synced folders
  If outputDir is in iCloud Drive, a cloud storage folder, Desktop or
  Documents, reports may leave this Mac. scan and gk doctor warn about it.

Backups
  Time Machine copies these files like any others. Turn on backup encryption
  in Time Machine's settings to keep them private there too.`,
		},
	}
}
