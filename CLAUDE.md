# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

Build steps 1 to 7 of `docs/groundskeeper-plan.md` are done, so the Mac side is complete: CLI skeleton, logging, size engine, all Mac scanners, the HTML report, `clean browsers` (`internal/clean`), history with metrics (`internal/history`, `internal/metrics`), the `history`, `stats` and `logs` commands, the launchd schedule (`launchd/`), notifications (`internal/notify`), `doctor` (`internal/doctor`) and the help topics. Next is the iPhone spike (step 8). CLI tests keep scans inside a fake home through the `appDirs` and `toolRunner` hooks in `internal/cli/scan.go`. The plan is the source of truth. Read it before starting work, follow its **Build order** step by step, and update the plan if a decision changes.

After each build step, add an entry at the top of `docs/build-log.md`: what was built, decisions, problems found (with the evidence that proved the root cause), what was verified and how, and what is still open. Keep personal paths and data out of it.

Groundskeeper is a macOS CLI (`groundskeeper`, alias `gk`), written in Go, that scans the Mac (and an iPhone connected over USB) for disk space that can be reclaimed and writes an HTML report. launchd runs it on a schedule twice a week.

## Commands

- `make build`: builds `bin/groundskeeper` with the git version stamped in
- `make test`: `go test ./...`, then `govulncheck` (pinned in the Makefile). Run a single test with `go test ./internal/scan -run TestName`
- `make run`: builds, scans and opens the report
- `make install` / `make uninstall`: the binary in `~/bin`, the `gk` symlink, zsh completion for both names in `~/.zsh/completions`, and the LaunchAgent (loaded with `launchctl bootstrap`). Reinstall after changes to use them as `gk` and on the schedule
- `launchctl kickstart gui/$(id -u)/com.lot86.groundskeeper`: run the scheduled job now, as launchd runs it (stdin `/dev/null`, minimal `PATH`)
- `./bin/groundskeeper scan --dry`: scan this Mac without writing the report or log
- `go test -race ./...`: the size engine and scanners are concurrent

Still to come: `make install-tunnel` (step 9), and `swift build` for `photos-helper` (step 10).

Go is pinned to the latest patch release in `go.mod` (`go 1.27.3`). `GOTOOLCHAIN=auto` downloads it if needed. When `govulncheck` reports a vulnerability, upgrade the Go patch release or the module it names rather than suppressing it.

## Architecture (as planned)

- **Scanners** (`internal/scan/`, `internal/iphone/`) each implement `Scanner { Name(); Scan(ctx, home) (Findings, error) }`. `Findings` holds the skipped paths and `Item`s that carry a section, path, size in bytes, safety tag (`safe` | `review` | `keep`), and a cleanup **hint**. Hints are only displayed. They are never executed.
- **Size engine** (`internal/size/`): a worker pool with a timeout per folder. It counts allocated blocks (`Blocks * 512`), not apparent size, skips symlinks, and counts each hardlink once by device and inode. Paths it can't read are recorded as "skipped". They are not treated as errors.
- **Aggregation**: scanner results are combined with the previous snapshot (`internal/history/`, stored in `~/.local/state/groundskeeper/`) to work out deltas. The results then go to the HTML report (`internal/report/`, template embedded with `go:embed`), a snapshot JSON file, and a macOS notification.
- **CLI** (`internal/cli/`, one file per command): Cobra wrapped in Charm's Fang, with Lip Gloss for styling and Bubble Tea for spinners and the photo review screen. Commands are grouped in the help output as Report, Review, Clean and Setup. Running `groundskeeper` with no subcommand runs `scan`. Every command's help has a summary, a description, a safety note, examples, flags and a See also list. CLI tests enforce this.
- **Logging and metrics** (`internal/logging/`, `internal/metrics/`): `log/slog` writes JSON lines to `~/Library/Logs/groundskeeper.log`, and every field goes through a helper that only accepts safe values. Run and scanner metrics are stored in each snapshot and shown in the report's Run details section and the `stats`, `logs` and `doctor` commands. Nothing is sent off the Mac.
- **photos-helper** (`helpers/photos-helper/`): a Swift CLI for PhotoKit, ImageCaptureCore and Vision. The Go side calls it and exchanges JSON over stdout (commands `list`, `analyze`, `thumbnail`, `delete`).

## Safety invariants (do not break)

- The scan and the scheduled run are **read-only**. Only the explicit `clean` subcommands delete anything.
- Every `clean` command is a **dry run unless `--apply`** is given. `clean browsers` uses an allowlist, refuses to run while the browser is running, moves files to the Trash, and never touches site data, cookies, history or passwords.
- Photos are only ever deleted through PhotoKit or ImageCaptureCore, never by removing files from the phone's camera roll directly.
- The only files the tool deletes on its own are its own snapshots older than six months.
- Output is plain (no colors, spinners or boxes) when stdout isn't a TTY or `NO_COLOR` is set.

## Security and privacy (do not break)

Groundskeeper runs with Full Disk Access and reads browser profiles, app containers and the photo library. Treat everything it touches as sensitive.

- **Measure, never read contents.** Scanners only stat files and add up their sizes. Never open, parse or copy cookie, password, history, Local Storage, IndexedDB, keychain or Messages files. Reading files is allowed only where the plan calls for it: Ollama manifests, `pyvenv.cfg`, `Info.plist` and Hugging Face directory names.
- **No network access.** The tool makes no outbound requests: no telemetry, no update checks, no remote reports. The only external connection is go-ios over USB to the local iPhone.
- **Never leak secrets into output.** Reports, snapshots, logs, notifications and `photos-helper` JSON contain paths, sizes and dates only. Never include file contents, environment variables, tokens, or command output that might hold them.
- **Private file permissions.** Create the state, report and log directories with `0700` and their files with `0600`. Reports list paths that reveal project, app and account names.
- **Escape the HTML report.** Render it with `html/template`, never `text/template` or string concatenation, because file and folder names are attacker-controllable. See https://pkg.go.dev/html/template.
- **No shell.** Run external tools (`git`, `mdls`, `brew`, `pgrep`, `podman`, `osascript`, `photos-helper`) with `exec.CommandContext` and separate arguments, never `sh -c`. Pass values such as notification text as arguments, not as interpolated AppleScript source. See https://pkg.go.dev/os/exec.
- **Shell-quote paths in hints.** Suggested commands are copied and pasted by the user, so paths must be quoted to stop a crafted file name from injecting commands.
- **Validate before deleting.** `clean` commands resolve symlinks (`filepath.EvalSymlinks`) and confirm the real path is still inside the allowlisted root before moving anything. Bundle IDs and asset IDs are checked against the last snapshot or review selection. Arbitrary input is never accepted.
- **No encryption at rest, by design.** FileVault covers it. `doctor` warns if FileVault is off, and `scan` and `doctor` warn if `outputDir` is in a synced folder (iCloud Drive, CloudStorage, Desktop, Documents).
- **Scrubbed fixtures only.** Recorded go-ios responses go through the recorder's scrubber, which removes device IDs, MAC addresses, phone numbers, emails, device names, third-party bundle IDs and file names, before anything is written to `testdata/`. `photos-helper` fixtures are written by hand. A guard test fails if `testdata/` contains anything that looks like real device or personal data.
- **Least privilege.** Only the iOS 17 tunnel needs root, and it is a separate LaunchDaemon. The main binary never asks for `sudo`.
- **Dependencies:** keep them to those named in the plan, and run `govulncheck ./...` before releases (https://go.dev/doc/tutorial/govulncheck).

## Testing approach

Tests use a fake home directory created with `t.TempDir()`. iPhone tests replay recorded go-ios responses and canned `photos-helper` JSON, so they run without a phone attached.

## Library versions

Pin the latest stable versions. The Charm libraries (Fang, Lip Gloss, Bubble Tea, Bubbles) are moving to v2 APIs, so check their release notes before using them.
