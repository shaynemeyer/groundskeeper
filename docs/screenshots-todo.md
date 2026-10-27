# Terminal screenshots to take for the user guide

Save each one to `docs/assets/` with the file name shown. Once they're there, they get their metadata stripped (`magick mogrify -strip -define png:exclude-chunk=date,time`), are checked by eye for personal details (the guide tests only check text, not images), and are placed in [the user guide](user-guide.md).

**Setup for every shot:** Ghostty in dark mode, about 100 columns wide, Retina capture (Cmd-Shift-4, then Space for a whole window), to match the report screenshots. Several commands print your real home folder, apps or photos; each item says what to watch for. Crop anything personal, or take the shots in a separate demo macOS user account.

## Install and first run

- [ ] **`terminal-doctor.png`**: `gk doctor`, ideally with every line `ok`. Goes under "Check the setup".
  - Prints full paths with your user name (`/Users/<you>/bin/groundskeeper`). A demo user account is cleanest; otherwise crop or blur them.
- [ ] **`terminal-scan.png`**: `gk scan --dry`, the summary. Goes under "Your first scan".
  - "Largest items" lists your real apps and folders. Crop to the first three lines, or use the demo account.
- [ ] **`settings-full-disk-access.png`**: System Settings > Privacy & Security > Full Disk Access, with your terminal and `groundskeeper` switched on. Goes under "Give it Full Disk Access".
  - The pane lists every app you've granted access; crop to those two rows.

## Cleaning up the Mac

- [ ] **`terminal-clean-browsers.png`**: a `clean browsers` dry run. Goes under "Clear browser caches".
  - A running Chrome is skipped, so use a made-up Brave profile:
    ```sh
    H=$(mktemp -d)
    mkdir -p "$H/Library/Caches/BraveSoftware/Brave-Browser/Default/Cache" "$H/Library/Caches/BraveSoftware/Brave-Browser/Default/Code Cache" "$H/Library/Application Support/BraveSoftware/Brave-Browser/Default"
    head -c 5000000 /dev/zero > "$H/Library/Caches/BraveSoftware/Brave-Browser/Default/Cache/d"
    head -c 2000000 /dev/zero > "$H/Library/Caches/BraveSoftware/Brave-Browser/Default/Code Cache/d"
    HOME=$H gk clean browsers --browser brave
    ```
  - It prints the temporary folder's path (`/var/folders/...`); harmless, but you can crop it.

## The schedule

- [ ] **`notification.png`**: the "Rook's report is ready" banner. Goes under "The notification".
  - Trigger it with `gk scan --notify` or `launchctl kickstart gui/$(id -u)/com.lot86.groundskeeper`. It only shows free space and sizes.

## The iPhone

- [ ] **`terminal-icloud-question.png`**: the iCloud Photos question. Goes under "Where your photos are read from".
  - Run `gk setup photos`, take the shot, then press **esc** so the saved answer isn't changed. Nothing personal shows.
- [ ] **`terminal-clean-iphone-apps.png`** (optional): `gk clean iphone-apps <bundle ID>` without `--apply`. Goes under "Remove an app".
  - Shows the app's name and bundle ID; pick one you don't mind showing.

## Duplicate photos

- [ ] **`terminal-review-photos.png`**: `gk review photos` in Ghostty with inline thumbnails, Keep and Extra showing. Goes under "Review the duplicates". The most useful one.
  - Shows your actual photos. Take a few throwaway near-identical shots first (a mug, a plant), run `gk iphone`, and capture that group.
- [ ] **`terminal-clean-photos.png`**: the `gk clean photos --apply` confirmation ("Type 2 to delete them:"). Goes under "Delete the extras".
  - **Take the shot, then press Return without typing anything.** It answers "Nothing was deleted." On the iPhone, deletes are permanent.

## Trends

- [ ] **`terminal-stats.png`** (optional): `gk stats`. Goes under "See how your disk changes".
  - Shows scanner names, timings and counts; its skipped paths can show real paths, so check before saving.
- Skip `gk history`: it lists your real fastest-growing items by path, and the report screenshot already shows Growing fastest from made-up data.
