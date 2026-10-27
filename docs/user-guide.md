# Groundskeeper user guide

Groundskeeper (`groundskeeper`, or `gk` for short) finds disk space you can get back on your Mac, and on an iPhone plugged in over USB. Twice a week it scans and writes an HTML report. The report lists what's taking space and suggests a command for each item. It never deletes anything by itself: you decide what goes, and only `gk clean` commands remove anything.

This guide walks through using it, task by task. For every flag of a command, run `gk help <command>`.

## Install and first run

### What you need

- **macOS 26 or later.** The photo helper needs it.
- **The Xcode Command Line Tools,** for Swift 6.2. If you don't have them, run this and follow the prompt:
  ```sh
  xcode-select --install
  ```
- **Go,** from [go.dev/dl](https://go.dev/dl/) or Homebrew (`brew install go`). Any recent version works: the build downloads the exact version Groundskeeper needs.

There's no app or installer download. You build Groundskeeper from its source code, which takes a minute or two.

### Build and install

```sh
git clone https://github.com/shaynemeyer/groundskeeper.git
cd groundskeeper
make install
```

`make install` builds Groundskeeper and puts these in place:

| What | Where |
|---|---|
| The program, and `gk` as a short name for it | `~/bin/groundskeeper`, `~/bin/gk` |
| The photo helper, used for iPhone and Mac photos | `~/bin/photos-helper` |
| Tab completion for zsh | `~/.zsh/completions/_groundskeeper` |
| The schedule (a LaunchAgent) | `~/Library/LaunchAgents/com.lot86.groundskeeper.plist` |

It doesn't need `sudo` and asks for no password. To install a newer version later, run `git pull` and `make install` again in the same folder.

### Let your shell find gk

If `gk --version` says "command not found", `~/bin` isn't on your `PATH`. Add it, and turn on tab completion, by adding these lines to `~/.zshrc`:

```sh
export PATH="$HOME/bin:$PATH"
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit
```

Open a new terminal window, then check:

```sh
gk --version
```

### Give it Full Disk Access

macOS keeps some folders private (Safari, Mail, some browser data, and sometimes Downloads) unless an app has Full Disk Access. Without it, Groundskeeper still works but lists those folders under **Skipped paths** in the report instead of measuring them.

Open **System Settings > Privacy & Security > Full Disk Access** and add two things:

1. **Your terminal app** (Terminal, Ghostty, iTerm2 and so on), for scans you start yourself.
2. **`~/bin/groundskeeper`,** for the scheduled scans. Click **+**, press **Command-Shift-G**, type `~/bin/groundskeeper` and press Return.

Groundskeeper only measures file sizes. It never opens or reads the contents of private files such as cookies, passwords, history or messages. See [Privacy and your data](#privacy-and-your-data).

### Tell it where your code lives

Groundskeeper looks for old build output (`node_modules`, `target`, virtual environments and so on) inside your project folders. By default it looks in `~/projects`, `~/github`, `~/code` and `~/Developer`. If your code is somewhere else, create `~/.config/groundskeeper/config.json` with your folders:

```json
{
  "projectRoots": ["~/src", "~/work"]
}
```

Every other setting has a default; see [Configuration](#configuration).

### Check the setup

```sh
gk doctor
```

`doctor` checks everything above and says how to fix anything that's wrong. Each line starts with `ok`, `warn` or `fail`:

| Check | What it means |
|---|---|
| Installed binary | `~/bin/groundskeeper` exists. If not, run `make install`. |
| Photos helper | `~/bin/photos-helper` exists. Only photo scanning needs it. |
| Photos access | Whether your terminal app can read the Mac Photos library. Only matters if you scan photos on the Mac; see [Duplicate photos](#duplicate-photos). |
| Full Disk Access | Whether private folders can be measured. |
| Schedule | The LaunchAgent is installed and loaded, and when it runs next. |
| Last scheduled run | When the schedule last ran and whether anything failed. "None yet" is normal right after installing. |
| FileVault | Whether your disk is encrypted. Reports list folder and app names, so FileVault should be on. |
| Report folder | Reports are saved somewhere that doesn't sync to the cloud. |
| Log files | The logs can be read only by you. |

A `warn` is worth reading but doesn't stop anything. A `fail` means something won't work until you fix it.

### Your first scan

```sh
gk --open
```

This scans your Mac, which takes from a few seconds to a few minutes, then opens the report in your browser. In the terminal you'll see a short summary like this:

```
Free space   123.1 GB of 494.4 GB
Reclaimable  18.4 GB (safe items)
Report       ~/Reports/groundskeeper/2026-10-27.html

Largest items:
     12.6 GB  safe    Homebrew
      4.2 GB  review  Ollama: llama3:8b
      3.1 GB  safe    projects/demo-app/node_modules
```

Running `gk` with nothing after it does the same as `gk scan`. To see the summary without writing a report, run `gk scan --dry`.

## Reading the report

Each report is saved as `~/Reports/groundskeeper/<date>.html`, and `latest.html` in the same folder is always the newest one. Old reports aren't deleted for you; remove them from that folder whenever you like.

### The top of the report

- **Free space:** how much of your disk is free right now.
- **Reclaimable (safe):** the total of the items tagged **safe**. This is space you can get back with no downside.
- **Since the last scan:** how free space changed, once there's an earlier scan to compare with.

### Sections

Items are grouped by what they are:

| Section | What it lists |
|---|---|
| Caches | Package manager caches (npm, Homebrew, pip, Go, Cargo and others) and the largest folders in `~/Library/Caches` |
| AI models | Ollama models, Hugging Face downloads, LM Studio and ComfyUI models |
| Build artifacts | `node_modules`, `target`, `.next`, virtual environments and similar folders in your projects |
| Toolchains | Rust toolchains, Node versions (Volta), Python versions (pyenv, uv) |
| Containers | Podman's virtual disk, and images and volumes inside it you could prune |
| Downloads | Files in `~/Downloads` older than 90 days |
| Browsers | Each browser's caches and offline data |
| Unused apps | Apps you haven't opened in 120 days, with their leftover data |

### Safety tags

Every item has a tag:

- **safe:** deleting it costs nothing; the tool that made it rebuilds it when needed. Caches, and build output in projects you haven't touched in 90 days, are safe.
- **review:** probably removable, but look first. A model you might still use, a project you're working on, an app you rarely open.
- **keep:** shown so you know where the space is, not as a suggestion. For example, a browser's cookies and site data.

### Suggested commands

Next to most items is a command that removes it, such as `npm cache clean --force` or `ollama rm 'llama3:8b'`. Groundskeeper never runs these. Copy one into your terminal if you want it. File names in the commands are quoted, so even a strangely named folder is passed as a single name and can't run anything else. See [Cleaning up the Mac](#cleaning-up-the-mac).

### Changes since the last scan

From the second scan on, the report has a **Change** column showing how much each item grew or shrank, and a **Growing fastest** list of the items that grew most. Use it to spot something filling your disk slowly.

### Skipped paths

Folders Groundskeeper couldn't measure, and why:

- **permission:** macOS didn't allow it. Give Full Disk Access as described in [Install and first run](#give-it-full-disk-access).
- **timeout:** measuring the folder took more than two minutes, so it was skipped. This can happen with folders holding millions of files.

### Run details

How long each part of the scan took, and how much it found. A part that was slow, timed out or failed is highlighted. You don't need this day to day, but it helps explain an incomplete report.

## Cleaning up the Mac

Groundskeeper suggests; you decide. There are two ways to act on the report.

### Run a suggested command yourself

For most items, copy the command next to it in the report and paste it into your terminal. For example, to clear npm's cache:

```sh
npm cache clean --force
```

Read the command before you run it. Items tagged **review** deserve a second look: `ollama rm` deletes a model you'd have to download again, and an `rm -rf` on a project's `node_modules` means running `npm install` before you work on it next. Commands that use `rm -rf` delete straight away; they don't use the Trash.

### Clear browser caches

Browser caches are the one kind of Mac cleanup Groundskeeper does for you, because doing it by hand risks deleting the wrong folder. First see what it would do:

```sh
gk clean browsers
```

```
      5.0 MB  Brave · Cache                would move
             ~/Library/Caches/BraveSoftware/Brave-Browser/Default/Cache
      2.0 MB  Brave · Cache                would move
             ~/Library/Caches/BraveSoftware/Brave-Browser/Default/Code Cache
Dry run: nothing was changed. Add --apply to move 7.0 MB to the Trash.
```

Nothing changes until you add `--apply`. Then it moves the caches to the Trash straight away, without asking again:

```sh
gk clean browsers --apply
```

Things to know:

- **Quit the browser first.** A running browser is skipped, even in the dry run, because removing a cache it has open can corrupt it. The output says which browser to quit.
- **Pick browsers** with `--browser`, for example `gk clean browsers --browser chrome,firefox`. Without it, every installed browser is included.
- **Your browsing data is never touched:** cookies, logins, history, bookmarks and site data stay. The browser just downloads pages and images again as you visit them, so the first visits may be a little slower.
- **Offline data,** such as cached web apps and Chrome's on-device AI model, is left alone unless you add `--include-offline`. Sites that work offline will need a connection the next time you open them.
- If a folder can't be fully read (usually Safari's, without Full Disk Access), it's refused rather than partly moved.

### Get the space back

Moving something to the Trash doesn't free any space until you **empty the Trash** (Finder > Empty Trash). Until then you can undo it by dragging the folder out of the Trash back to where it was; the dry run's output shows the original location. The next report notes how much the clean commands moved to the Trash since the last one.

Every clean is written to the log; see it with `gk logs --cmd clean`.

## The schedule

`make install` sets up a scheduled scan every **Monday and Thursday at 9:00**. If your Mac is asleep then, the scan runs when it wakes. It only ever runs the read-only scan, never a clean command.

### The notification

When a scheduled scan finishes, you get a notification like:

> Rook's report is ready: 118.0 GB free, about 35.0 GB to tidy.

Rook is Groundskeeper's raccoon mascot. "To tidy" is the reclaimable total from the report. If something went wrong, the notification adds a line such as "2 scanners had problems; see the report." That means part of the scan failed or timed out, so the report is incomplete; [Run details](#run-details) and `gk logs --run last --level warn` say why. "3 new paths couldn't be read" means folders were skipped this time that weren't before, often because Full Disk Access was removed.

The notification is sent through macOS's Script Editor, so it shows Script Editor's icon. If you don't see any, check that **System Settings > Notifications > Script Editor** is allowed. Scans you start yourself don't notify unless you add `--notify`.

### Run it now

To run the scheduled scan immediately, exactly as the schedule runs it:

```sh
launchctl kickstart gui/$(id -u)/com.lot86.groundskeeper
```

`gk doctor` shows whether the schedule is loaded, when it runs next and when it last ran.

### Change the days or time

Edit `launchd/com.lot86.groundskeeper.plist` in the folder you cloned. Under `StartCalendarInterval`, each entry has a `Weekday` (1 is Monday, 0 or 7 is Sunday), an `Hour` and a `Minute`. Then run `make install` again to load the change. `gk help schedule` has the details.

### Turn it off

```sh
launchctl bootout gui/$(id -u)/com.lot86.groundskeeper
```

This pauses the schedule until you next log in, restart or run `make install`, because macOS loads it again at login. To turn it off for good, remove Groundskeeper with `make uninstall`; see [Remove Groundskeeper](#remove-groundskeeper).
