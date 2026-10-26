# Build log

One entry per build step from [the plan](groundskeeper-plan.md): what was built, decisions made, problems found and how they were proven, and what is still open. Newest first.

## Step 5: `clean browsers`

### Built

- `groundskeeper clean browsers [--browser chrome,firefox] [--include-offline] [--apply]`, in a new "Clean (dry run unless --apply)" help group, with help covering safety, examples and flags.
- `internal/clean`: acts only on the folders the Browsers scanner lists for the cache tier, plus the offline tier with `--include-offline`. Site data is never selected, and no flag can include it.
- Every folder gets an outcome (would move, moved to Trash, refused, failed), printed and logged with `dry_run`. Running browsers are reported with a message to quit them.
- The scan package exports `BrowserFolderList`, `BrowserKeys`, `BrowserRoots` and `RunTool`, so scan and clean share one definition of each tier.
- Log lines now carry the full command name (`clean browsers`), not just `browsers`.

### Decisions

- **Running check:** uses `ps -axo comm=` and matches the browser's executable name, instead of `pgrep`. `pgrep` returns exit code 1 both for "no match" and in ways that are hard to tell apart from real errors behind the runner interface. If `ps` fails, nothing is cleaned.
- **Trash:** goes through macOS's `/usr/bin/trash -s` (macOS 15+), which uses the system Trash API, rather than renaming into `~/.Trash`. With `-s` a failed move exits non-zero (5 in testing), which is reported as `failed`.
- **Safety check (`allowed`):** the path must be clean and absolute, must not itself be a symlink, and its resolved location must be inside a resolved browser root. Symlinked profile folders are never followed, because discovery only takes real folders.

### Problems found and fixed

- **Would have moved folders it couldn't read.** A live dry run showed Safari's caches as "0 B would move": without Full Disk Access macOS blocks reading them, and the measurement's skipped paths were ignored. A failing test (a locked folder inside a cache) came first, then the fix: any folder with unreadable parts is refused with "grant Full Disk Access to clean it".
- **Test gaps found by mutation checks:**
  - Disabling the running check first produced a compile error, not a test failure, so the check was redone properly; it now fails `TestRefusesRunningBrowser`.
  - The symlinked-profile test was passing because discovery never follows that profile, not because of root containment. It was renamed to `TestSymlinkedProfileIsNeverFollowed`; containment is covered by the `allowed` unit test.

### Verified

- Mutation checks: removing the symlink check, root containment, clean-path check, running check, site-data exclusion, or the `--apply` gate each makes a test fail.
- Live dry run on a real Mac: Chrome and DuckDuckGo (running) were refused, Safari (no Full Disk Access) refused, and an unknown browser name gives the list of valid ones.
- End to end with the real `/usr/bin/trash`, in a throwaway home with a fake Firefox profile: the folder moved to the Trash, and the summary and log line were correct.

### Open

- `--browser` shell completion is planned for step 7.
- Nothing has been applied to real browser data yet; only dry runs and the throwaway-home test.

## Step 4: Containers, Downloads, Browsers and Unused apps scanners

### Built

- **Browsers:** every profile of Chrome, Edge, Brave, Arc, Chromium, Firefox, Safari and DuckDuckGo, in three tiers: Cache (safe), Offline data (review: Service Worker, Chrome's on-device AI model, crash reports) and Site data (keep, measured only, never suggested). The Caches scanner now leaves browser folders to this scanner, and opens shared vendor folders such as `Google` one level down so other Google caches are still counted.
- **Containers:** the Podman machine's disk image on the Mac (measured by allocated blocks), plus reclaimable images, containers and volumes from `podman system df --format json`. A stopped machine is never started.
- **Downloads:** top-level entries that arrived more than `staleDays` ago, by creation time.
- **Unused apps:** last-opened date from `mdls`, plus activity in the app's own data. The size includes leftovers, and the hint depends on how the app was installed: Homebrew cask, App Store, vendor uninstaller, or Trash. Apple apps, running apps and `appExclude` are never listed.
- Shared tool lookup (`tools.go`) that falls back to `/opt/homebrew/bin`, `/usr/local/bin` and `/opt/podman/bin`, because launchd's `PATH` is only `/usr/bin:/bin:/usr/sbin:/sbin`.

### Decisions

- Service Worker data is in the review tier. The plan's table and tier description disagreed, so I followed the tier description and fixed the table.
- Downloads use creation time (birthtime), because downloads often keep the server's modification time.
- Podman volume hint is `podman volume prune` without `-a`, because named volumes can hold data. The note explains `-a`.
- `brew` runs with `HOMEBREW_NO_AUTO_UPDATE=1`, so a scan never triggers a network update.

### Problems found and fixed

- **Apps used recently were flagged as unused.** On a real Mac, iTerm, OBS, DBeaver and DevToys were listed. Root cause: the second signal didn't check `Preferences/<bundle ID>.plist`, and iTerm keeps its data in `iTerm2`, not `iTerm`. Proof: their preference files had changed 45 to 96 days earlier, inside the 120-day window. A failing test came first, then the fix. Only an app with 253-day-old preferences is still listed.
- **CLI tests scanned the real system.** With a fake home, the real `/Applications` made every app look unused, and the tests ran the real `podman`, `brew` and `mdls` (17 s, failing). The `appDirs` and `toolRunner` hooks in `internal/cli/scan.go` now keep tests hermetic (about 1 s).

### Verified

- Podman 6.1.1 JSON fields and prune flags, from its own output and `--help`.
- `mdls -raw` prints NUL-separated values with `(null)` when unknown, checked on macOS.
- Homebrew cask JSON shape (`casks[].token`, `artifacts[].app`).
- Chromium's cache folder rule ([docs](https://chromium.googlesource.com/chromium/src/+/main/docs/user_data_dir.md)).
- On APFS, setting an older modification time also moves the creation time back, which the Downloads tests rely on.
- Mutation checks: disabling the data-activity signal makes its test fail.

### Open

- Arc's cache path is derived from the Chromium rule but wasn't checked on a real install.
- Browser hints name `groundskeeper clean browsers`, which arrives in step 5.
- Measuring other apps' `~/Library/Containers` folders might trigger macOS's "access data from other apps" prompt in a terminal without Full Disk Access. It wasn't seen, but hasn't been ruled out.
- A full scan takes about 25 s, mostly build artifacts (14 s) and caches (7 s).

## Step 3: AI models and toolchains scanners

### Built

- **Ollama:** one row per model, with size and pull date from its manifest (matches `ollama list`), plus one row for blobs no manifest uses.
- **Hugging Face:** one row per repo in the hub cache as `model/org/name`, with an `hf cache rm` hint.
- **LM Studio:** models in `~/.lmstudio` and the older `~/.cache/lm-studio`.
- **ComfyUI:** model folders in `~`, `~/Documents` and two levels into each project root.
- **Toolchains:** rustup, Volta, pyenv and uv, with an uninstall hint for each.
- Shared `measure` and `keep` helpers, and `size.Allocated`.

### Decisions

- Default tool locations only, because a launchd job doesn't see shell variables such as `OLLAMA_MODELS`. `extraPaths` is the planned escape hatch but isn't wired up yet.
- The orphan-blob check is skipped if Ollama's `manifests-v2` folder holds files, since those manifests aren't parsed and every blob would look unused.
- Newer Hugging Face versions symlink repo blobs into a shared store. Those files count toward each repo, matching `hf cache ls`; without this, such repos measured 0 bytes.
- The pyenv hint omits `-f`, so pyenv still asks for confirmation. Volta has no uninstall for Node versions (and is unmaintained since November 2025), so its hint is `rm -rf`.

### Verified

- Hint syntax from the [Hugging Face cache docs](https://huggingface.co/docs/huggingface_hub/guides/manage-cache), the [uv Python versions docs](https://docs.astral.sh/uv/concepts/python-versions/) and `--help`, the [pyenv man page](https://dyn.manpages.debian.org/trixie/pyenv/pyenv-uninstall.1.en.html), and `rustup toolchain uninstall --help`.
- Ollama 0.40 still uses the classic manifest format; checked against a real install, including MLX models with over 1,000 tensor blobs.
- Mutation checks: disabling the shared-store count or the `manifests-v2` guard makes their tests fail.

## Step 2: Build-artifact scanner

### Built

- Detects `node_modules`, `.next`, venvs (with `pyvenv.cfg`), Rust `target` (beside `Cargo.toml`), .NET `bin`/`obj` (beside a `.csproj`), and `__pycache__` totalled per project. Skips `.git` and `excludePaths`.
- Staleness from the last git commit, otherwise the newest file outside artifact folders. Items 90 days old or more are tagged safe.
- Broken venvs: `pyvenv.cfg` points at a Python that no longer exists. Always listed, even when small.

### Decisions

- A project is the top-level folder under a project root, so a monorepo's nested artifacts share its staleness.
- `git` runs only when that folder has its own `.git` (so a dotfiles repo in `~` is never used), and with `core.fsmonitor=false`.
- `--dry --verbose` prints debug lines to stderr without writing files.

### Problems found

- A test venv measured 0 bytes because empty files use no blocks on APFS. The fix was to the test data, not the scanner.

### Verified

- On a real Mac: 80 artifact folders over 100 MB, 63 stale, and 12 broken venvs left behind by removed pyenv and Homebrew Pythons.

## Step 1: CLI skeleton, logging, size engine, Caches scanner and HTML report

### Built

- **Size engine:** allocated blocks, symlinks skipped, hardlinks counted once, 6 workers, a timeout per folder, and unreadable paths recorded as skipped.
- **Caches scanner:** package-manager caches plus the other folders in `~/Library/Caches`, with shell-quoted hints.
- **Config:** loaded over built-in defaults.
- **Logging:** `slog` JSON lines with typed fields only, errors logged by kind, rotation at 5 MB keeping 3 files.
- **HTML report:** embedded `html/template`, no network loads, `0600` files in a `0700` folder.
- **CLI:** Cobra with Fang, and `scan` as the default command.
- **Makefile:** `build`, `test` (with govulncheck) and `run`.

### Decisions

- `Scan` returns `Findings` (items plus skipped paths) instead of `[]Item`, so skipped paths reach the report.
- The scan spinner is deferred until there are enough scanners to need it.

### Problems found and fixed

- **govulncheck found reachable vulnerabilities:** two `html/template` escaping bugs in Go 1.27.1 and an infinite loop in `golang.org/x/text`. Fixed by pinning Go 1.27.3 and `x/text` v0.41.0.

### Verified

- Library versions looked up with `go list -m`, and the Fang v2 and Lip Gloss v2 APIs read with `go doc` before use.
- On a real Mac: report and log written with private permissions, and macOS-protected folders recorded as skipped.
