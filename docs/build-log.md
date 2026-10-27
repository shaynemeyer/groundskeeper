# Build log

One entry per build step from [the plan](groundskeeper-plan.md): what was built, decisions made, problems found and how they were proven, and what is still open. Newest first.

## Step 15: Mac cleanup (in progress)

### Built

- **Increment 1:** `scan.Item.Trash` is the one path whose move to the Trash is the whole cleanup for a row. The Caches scanner sets it on known caches and the other `~/Library/Caches` folders, Build artifacts on each artifact folder (not the `__pycache__` totals), Downloads on each entry, and AI models on Hugging Face `models--*` and `datasets--*` repos and LM Studio model folders. Ollama, ComfyUI, Toolchains, Containers, Unused apps and Browsers never set it. Snapshot rows (`history.Item`) now store `trash`, `safety` and `note`, so `review mac` can show them from the last snapshot.
- **Increment 2:** the allowlist and the saved review (`internal/clean/mac.go`). `MacRules.Check` refuses a path that isn't clean or absolute, isn't inside the home folder, is gone, is a symlink, goes through a symlink (`filepath.EvalSymlinks` must give the same path), has changed between file and folder, or fits no rule for its section. The section rules call `scan.CacheTrashable`, `scan.ArtifactTrashable` (inside a configured project root) and `scan.HFTrashable` (`internal/scan/trash.go`), so the scanner and the clean-time check share one definition; Downloads must be a non-hidden direct child of `~/Downloads`, and LM Studio a folder two levels under either models folder. `NewMacReview` builds a `MacReview{Time, Items}` from ticked snapshot rows, refusing rows without `trash` and rows whose path is gone; `SaveMacReview` and `LoadMacReview` keep it in `~/.local/state/groundskeeper/mac-review.json` (`0600`), and `Without` drops moved items.
- **Increment 3:** `gk clean mac` (`internal/cli/clean_mac.go`). It loads the saved review and prints the count, total and review date; without `--apply` it stops there. With `--apply` it asks `Move them to the Trash? [y/N]`, then `clean.Mac.Clean` checks each item with `MacRules` (project roots from the config) and moves it with `/usr/bin/trash -s`. Moved items leave the saved review; refused and failed ones stay and are listed with the reason. The run is saved as a `metrics.Clean` with command `clean mac`, which the report and `gk stats` already count, and each item is logged. The user guide gained "Move ticked items to the Trash", and no longer says browser caches are the only cleanup Groundskeeper does.

### Decisions

- **Hugging Face repos that link into the shared `<hub>/blobs` store get no `Trash`.** Their size counts the shared files they symlink to (step 3), and trashing the repo folder removes only the links, so it wouldn't free what the row reports. `TestTrashModels` covers it, and fails when the check is removed. The plan's table and allowlist now say so.
- **The review records each item's kind (`dir`).** The plan checks "the same kind as at scan time", but snapshots don't record it. The review records it with `os.Lstat` when saved, at most 7 days after the scan.
- **A project root that is itself a symlink makes its artifacts unselectable.** Their paths go through the symlink, so `Check` refuses them. This follows the plan's rule as written; if it bites, the fix is to resolve roots in the config, not to relax the check.
- **A dry run of `clean mac` neither checks items nor records metrics,** as the plan describes and as `clean photos` does. The checks run at `--apply`, right before each move, so their answer is current.
- **The CLI tests' fake home is resolved with `filepath.EvalSymlinks`.** `t.TempDir()` is under `/var`, a symlink to `/private/var`, so every path in a test review went through a symlink and was refused. No other test depended on the unresolved path.
- **Both LM Studio locations are trashable,** `~/.lmstudio/models` and the older `~/.cache/lm-studio/models`, since the scanner lists both. The plan's allowlist now names both.

### Verified

- `go test -race ./...` passes. New tests: `internal/scan/trash_test.go` (which rows get `Trash` and which never do) and `TestSnapshotRecordsTrashAndSafety` (a scan's snapshot keeps the npm row's `trash` and `safety`), and `internal/clean/mac_test.go` (each section's rule accepted, 19 refusals with their reasons, the review round trip and `0600`). Removing the through-a-symlink check makes `TestMacRulesRefuse` fail. `TestMacCleanMovesOnlyAllowedItems` covers moved, refused and failed; `internal/cli/clean_mac_test.go` covers no review, the dry run, declining, and `--apply` moving npm while a hand-edited item pointing at `~/Documents` is refused, kept on disk and kept in the review, with the clean snapshot's counts.

### Open

- Increments 4 and 5: `review mac` (until then `clean mac` points at a command that doesn't exist yet), and the security tests.

## Step 17: user guide

### Built

- **`docs/user-guide.md`,** written for anyone who finds the repository: install and first run, reading the report, cleaning up the Mac, the schedule, the iPhone, duplicate photos, trends and troubleshooting, privacy (with removing Groundskeeper), and configuration. It shows the common commands and points to `gk help <command>` for every flag.
- **Sync tests** (`internal/cli/guide_test.go`). `TestGuideCommandsParse` runs every `gk` line in the guide's code blocks through Cobra's `Find` and `ParseFlags` without executing it, with the help command and the `--version` flag Fang adds. `TestGuideMentionsEveryCommand` requires each visible command (`gk <path>`) and help topic (`gk help <topic>`). `TestGuideDocumentsEveryConfigKey` requires every `config.Config` JSON key in the Configuration section. `TestGuideHasNoPersonalData` runs `fixtures.Check` and rejects `/Users/` and `/home/` paths.
- **The README** lost its out-of-date "Commands (planned)" table (it listed a `config` command that doesn't exist) for a short Get started that links to the guide, and gained a Development block and a link in Docs.

### Decisions

- **The tests were committed with the last section,** not first: they fail until the guide is complete, and `main` stays green. The guide's sections were committed one at a time.
- **The LaunchAgent label is allowed in the personal-data check.** `fixtures.Check` flags any bundle ID outside `com.apple.*` and `com.example.*`, so `com.lot86.groundskeeper.plist` tripped it; the test removes `doctor.Label` first. A third-party ID (`com.spotify.client`) added to the guide still failed it.
- **Sample output is written by hand in the tool's format.** Even against a fake home, `scan` measures the real `/Applications`, so its Largest items would list this Mac's apps. The `clean browsers` sample came from a fake home with a made-up Brave profile.

### Problems found

Checking the guide's claims against the code before each commit found six that were wrong, all fixed before committing:
- Reports aren't kept for six months: only snapshots are pruned (`history.Prune` in `scan.go`); reports accumulate.
- Every report section shows its heading, even when empty (`template.html`).
- `launchctl bootout` doesn't stop the schedule until the next `make install`: macOS loads `~/Library/LaunchAgents` again at login.
- Only apps listed as probably unused come with a ready-made `clean iphone-apps` command; every app shows its bundle ID.
- The log isn't "kept to a few MB": it rotates at 5 MB and keeps three old files.
- The plan said macOS 15; `photos-helper`'s `Package.swift` requires macOS 26 (`platforms: [.macOS(.v26)]`) and Swift tools 6.2.

`TestGuideMentionsEveryCommand` also found that the guide never mentioned `gk help safety`. The tests were first checked against a throwaway guide: they caught exactly the unknown command, unknown flag and unknown help topic, passed `--version`, `--help`, `help safety` and a line with a trailing comment, listed the 13 config keys left out, and flagged a home path. A script checked that all eight internal links resolve to headings.

### Verified

- `make test` passes, `gofmt -l .` is clean, and `govulncheck` reports no vulnerabilities that affect the code.

### Visuals

- **Two Mermaid diagrams:** how Groundskeeper works (schedule or you, read-only scan, report and notification, you decide) at the top, and the duplicate-photo flow with its iCloud branch. Both were rendered with Mermaid 12.2.0 in headless Chrome to check they parse; "gk clean photos --apply" wrapped mid-flag, so the label breaks before `--apply`.
- **Four report screenshots** in `docs/assets/`: the top with Growing fastest and Caches, the iPhone apps with Probably unused, Photos, and Skipped paths with Run details. They come from `docsSample` in `internal/report/docs_sample_test.go`, a made-up report (`/Users/you`, `com.example.*`, invented sizes) built on the existing test samples, with the escaping fixture's `<script>` file name replaced. To regenerate: `GK_DOCS_SAMPLE=<dir> go test ./internal/report -run TestWriteDocsSample -count=1`, screenshot `<dir>/report.html` with headless Chrome at `--force-device-scale-factor=2 --window-size=1100,3700`, crop with ImageMagick (`-crop 2200xH+0+Y +repage`), and strip metadata with `magick mogrify -strip -define png:exclude-chunk=date,time`. They show the dark theme, as this Mac was in dark mode.
- `sips --cropOffset` was ignored and every crop came from the middle of the page, seen in the first crops (the "top" crop showed the iPhone table); ImageMagick's `-crop` with an explicit offset worked.
- The PNGs were checked for EXIF, XMP and text chunks; `-strip` alone left a `tIME` chunk, which `png:exclude-chunk` removes.

### Command Line Tools build, verified

`DEVELOPER_DIR=/Library/Developer/CommandLineTools swift build -c release --package-path helpers/photos-helper --scratch-path <dir>` built `photos-helper` on a Mac that also has Xcode. The verbose build used only `CommandLineTools/SDKs/MacOSX27.0.sdk`, with no reference to `Xcode.app`; the binary runs and links Photos, ImageCaptureCore and Vision (`otool -L`). This was Command Line Tools 27 (Swift 6.4) on macOS 27, so the guide's minimum, macOS 26 with Swift 6.2, is still untested.

### Open

- The notification's Script Editor icon and the setting to allow it come from the plan, not from a check on a fresh Mac.
- The internal links are checked by a script run by hand, not by a test.
- There are no screenshots of the terminal screens (`gk doctor`, `gk review photos`); the guide shows their text instead.

## Step 14: logging and metrics tests

### Built

- **An audit of the plan's five items.** Three were covered: rotation keeping three files (`TestRotationKeepsThreeFiles`), `logs` filtering by level, run and command (`TestReadFilters`, and `TestLogsFiltersAndJSON` through the CLI), and `stats --json` matching the snapshot byte for byte (`TestStatsJSONMatchesSnapshot`). `TestSnapshotRecordsScannerMetrics` checked names, item counts and bytes but not skipped paths, and the notification didn't mention timeouts the way the plan said.
- **`TestSnapshotRecordsSkippedPaths`:** an unreadable folder in `.npm` gives the Caches scanner `Skipped: 1`, and the path is in the snapshot's skipped list.
- **`TestRotationStartsAtTheLimit`:** with a 50-byte limit, a 49-byte log stays and a 50-byte log rotates.
- **Timed-out scanners in every notification.** `metrics.Run.Problems()` counts scanners that failed or timed out on a folder, and the notification's "N scanners had problems" uses it in place of `Run.Errors()`. `notify.Summary.Failed` became `Problems`. `doctor` keeps `Errors()`, since it says "scanner(s) failed". `folderTimeout` in `internal/cli/scan.go` became a variable so `TestNotifyMentionsTimeoutsEveryRun` can force a timeout; it is still two minutes.

### Decisions

- **Timeouts are mentioned on every run, not only the first.** The plan's notification rule mentioned errors and paths skipped for the first time; a timed-out folder is recorded as a skipped path (`size.Skip` with reason `timeout`), so it was mentioned once, and never on a first run with no snapshot to compare with. A folder that times out every run leaves every report incomplete, so the user chose to mention it each time. The plan's notification rule now says so.
- **Scanner duration isn't asserted.** The skipped-paths test recorded `DurationMS: 0` for Caches on the fake home, so a non-zero check would fail at random.
- **A slow scanner alone isn't a problem** for the notification; Run details still highlights it.

### Problems found

- **No notification mentioned timeouts.** With `folderTimeout` at one nanosecond, two `scan --notify` runs both posted "Rook's report is ready: 123.4 GB free, about 0 B to tidy." with nothing about the timeouts: the first had no previous snapshot, and in the second the same paths were skipped last time. Root cause: the notification counted only `Error`, and timeouts only reached it through `NewSkip`.
- **Step 12's `report_test.go` wasn't gofmt-formatted.** Found by `gofmt -l` in this step and fixed in the same commit.

### Verified

- Each new test was checked by breaking the code: dropping `Skipped` from the scanner metrics, rotating at `<=` instead of `<`, and dropping the timeouts condition from `Problems` (which failed both `TestRunProblemsCountsFailuresAndTimeouts` and the CLI test).
- `make test` passes, `gofmt -l .` is clean, and `govulncheck` reports no vulnerabilities that affect the code.

### Open

- The scheduled run uses the installed binary; `make install` is needed for its notifications to mention timeouts.

## Step 13: security tests

### Built

- **An audit of the plan's 14 items against the existing tests.** Twelve were already covered by tests written alongside their features: symlink escape (`TestSymlinkedCacheIsRefusedAndTargetKept`, `TestSymlinkedProfileIsNeverFollowed`), path traversal (`TestAllowedRejectsDotDotAndOutsidePaths`), HTML escaping (`TestRenderEscapesFileNames`, `TestIPhoneReportEscapesPhotoIDs`), hint quoting (`TestShellQuote` and the scanners' hint tests), file permissions, the notifier's arguments (`TestSendPassesTextAsArguments`), iPhone and photo IDs (`TestIPhoneAppsRefuses`, `TestNewSelectionRefusesUnknownID`, and `TestCleanPhotosDeviceDeletes`, which checks the helper's stdin is exactly the saved review), pruning, FileVault, the synced output folder, the fixture scrubber and guard, and safe log fields. Safe log fields are enforced when the code compiles: `logging.Field` wraps an unexported `slog.Attr`, so only the helpers can make one, and nothing outside `internal/logging` logs through `slog`.
- **No content reads and no secrets now cover the snapshot.** `TestNoContentsOrSecretsInOutput` checked stdout, the report and the log, but not the snapshot the plan names. It now checks every file in the state folder for the marker, and fails if the scan wrote no snapshot.
- **No network, as two tests** (`cmd/groundskeeper/network_test.go`). `TestNoNetworkImports` runs `go list` on the module and fails if any of its packages imports `net`, `net/...`, `crypto/tls` or `golang.org/x/net`, or if any package but `internal/iphone` imports go-ios. `TestScanMakesNoConnections` builds the binary and runs a full `scan` against a temporary home under `sandbox-exec` with the profile `(deny network-outbound (with send-signal SIGKILL))`, so any connection, Unix sockets included, kills it. The scan must exit 0 and write its report. First, as a control, `iphone --dry` must be killed: it connects to usbmuxd, so this shows the sandbox catches this binary's connections.

### Decisions

- **The plan's "dialer that fails on any connection" isn't possible in Go.** There's no global dialer to replace; a test could only swap one for code that uses it. The binary also links `net`, `net/http` and `crypto/tls` through go-ios (`go list -deps`), so checking the binary's dependencies would always fail. The two tests above replace it, and the plan now says so.
- **The sandbox kills instead of only denying.** A denied connection returns an error the scan might log and carry on from, so the test could pass without noticing. Proven with curl to 127.0.0.1: without a deny rule it tried and got "connection refused" (exit 7); with the rule it was killed (exit 137, SIGKILL).
- **The strict profile denies Unix sockets too.** A full `scan` under it still exits 0 and writes the report and snapshot, so the scan opens no socket of any kind, and the stricter rule costs nothing.
- **The `iphone --dry` control stays in the test** because `sandbox-exec` is deprecated: if a future macOS stops enforcing the profile, the control fails instead of the scan check passing for the wrong reason.

### Verified

- Each new check was broken on purpose and failed, then reverted with `git checkout`: copying `GITHUB_TOKEN` into `Run.Command` (which only the snapshot stores) gave "marker in state file"; a `net/http` import in `notify` and a go-ios import in `cli` each failed `TestNoNetworkImports`; a `net.Dial` to 127.0.0.1 at the start of `runScan` gave "scan exited -1".
- `TestScanMakesNoConnections` takes about 6 seconds, and passes both inside the agent's sandbox and outside it.
- `make test` passes, with `govulncheck` reporting no vulnerabilities that affect the code.

### Open

- The sandboxed scan reads the real `/Applications` for the unused-apps scanner (read-only); the fake home only covers `$HOME`.
- `sandbox-exec` is deprecated. If it goes away, `TestNoNetworkImports` remains.
- There is no CI yet, so the plan's "in CI" doesn't apply; the sandboxed test runs in `make test`.

## Step 12: tests

### Built

- **An audit of the plan's list against the existing tests.** Each earlier step had added its own tests, and every item step 12 names already had them: artifact detection and staleness (`TestArtifactsDetectionRules`, `TestArtifactsStaleGitProjectIsSafe`), unused-app dating (`TestUnusedFlagsAppsUnchangedForDays`, `TestLastUsedDatesParsesMdlsOutput`), install-source detection (`TestAppsRemovalHintByInstallSource`), hardlink dedupe (`TestMeasureCountsHardlinksOnce`), Ollama manifests (`TestOllama*`), delta math (`TestDelta`, `TestHistoryListsScansAndGrowth`), iPhone parsing from scrubbed recordings and canned `photos-helper` JSON, every command's help (`TestEveryCommandHasHelpContent`), and no escape codes when stdout isn't a terminal (`internal/cli/cli_test.go`). The audit used test names and `go test -cover ./...` to find what wasn't covered.
- **The report's Photos section** (`internal/report`, 48.7% to 90.8%). `TestPhonePhotoTotals` checks `PhotoGroups`, `DuplicateBytes`, `OldScreenshots` and `Videos`: kept photos free nothing, sidecar bytes count, and screenshots report their first and last dates. `TestRenderShowsPhotos` checks the rendered rows and sentences.
- **`internal/metrics`** (0% to 100%). `TestScannerStatus` covers the one-minute boundary of `Slow` and each `Status`, with an error taking precedence over timeouts and slowness. `TestRunErrorsCountsFailedScanners` checks that timeouts alone don't count as failures, which `doctor` and `scan` rely on.

### Decisions

- **`internal/iphone/device.go` stays untested.** `Read`, `Uninstall` and their helpers are thin calls into go-ios over USB; what they return is parsed by `Parse`, which `parse_test.go` covers with scrubbed recordings. Faking go-ios would test the fake. The same goes for `iphone/record`, a command run by hand that writes only through the tested scrubber.
- **Every new test was checked by breaking the code.** Each passed on its first run, so the code under test was changed to be wrong (`PhotoGroups` counting `Bytes` without sidecars; `Slow` using `>` instead of `>=`), the tests were confirmed to fail, and the change was reverted with `git checkout`.

### Verified

- `make test` passes (`go test ./...` and `govulncheck`), and `go test -race ./...` passes.
- Coverage by package: clean 94.2%, cli 85.6%, config 77.4%, doctor 84.4%, fixtures 98.3%, history 89.5%, iphone 72.0% (the rest is `device.go`), logging 86.4%, metrics 100%, notify 100%, report 90.8%, scan 88.7%, size 93.2%, tui 94.9%, ui 80.5%.

### Open

- `govulncheck` reports one vulnerability in a required module that the code doesn't call. Looked into after step 17: it's GO-2026-5932, `golang.org/x/crypto/openpgp` being unmaintained and unsafe by design, affecting every `x/crypto` version with no fix (we're on the newest, v0.58.0). `x/crypto` comes in through go-ios and `software.sslmate.com/src/go-pkcs12` for `pbkdf2`, the only `x/crypto` package in the build (`go list -deps`), and nothing imports `openpgp`. Nothing to change; `CLAUDE.md` records it.
- The Mac library path of `clean photos` still hasn't run against a real library (step 11).

## Step 11: photo grouping, review and clean photos

### Built

- **Increment 1:** `internal/iphone/photos.go` runs `photos-helper` (`Helper{Path, Source, Run}`) and decodes `list` into `[]Asset` and `analyze` into `[]Analysis` plus `[]Failure`. An `{"error": kind}` answer becomes `ErrNoDevice`, `ErrLocked`, `ErrDenied` or `ErrTimeout`; a helper that dies without one is `ErrHelper`. `Distance` is Vision's feature-print distance, the squared Euclidean distance of the float32 vectors (step 10).
- **Increment 2:** `photos-helper hash --source <source>` reads asset IDs on stdin and writes `{"source","hashes":[{id, sha256}],"failed":[{id, reason}]}`, with the new reason `read`. The device path reads each file in 4 MB chunks with `ICCameraFile.requestReadData(atOffset:length:)`; the library path streams the original `PHAssetResource` (the one `list` counts as `bytes`, now shared as `originalResource`) through `PHAssetResourceManager.requestData` with network access off. `Helper.Hash` is the Go side.
- **Increment 3:** `NearDuplicates` (`internal/iphone/group.go`) sorts photos by date, compares every pair taken within the window and every pair in the same burst, and joins pairs at most `NearDistance` (0.025) apart with a union-find, so links chain. Groups hold two or more IDs, oldest first. Photos without an analysis are left out, and photos without a date are only compared within their burst.
- **Increment 4:** `ExactCandidates` (`internal/iphone/exact.go`) returns the assets that share their size and dimensions with another, the only ones worth hashing. `ExactDuplicates` groups assets whose size, dimensions and SHA-256 all match, oldest first; assets without a hash or a size are left out.
- **Increment 5:** `Keep` (`internal/iphone/best.go`) returns a group's keepers: every protected asset (`Asset.Protected`: favorite, edited or in an album) plus the best shot, the highest aesthetics score. With protected assets in the group the best shot is chosen among them. A missing score ranks lowest, and ties go to the older photo.
- **Increment 6, old screenshots and the largest videos.** First, device `list` folds each edited render (`IMG_E0001.JPG`) into its original (`IMG_0001.HEIC`) when exactly one original is in the same folder with the same creation date: the render is no longer an asset, its size goes to the original's `sidecarBytes`, and the original is `edited: true` (`deviceAssets` in `Asset.swift`). Then `list` gains `video` and `screenshot` on both sources: on the phone from the UTI (`public.movie`) and EXIF `UserComment` (`Screenshot`, from `requestMetadataDictionary`, metadata only), in the library from `mediaType` and the `photoScreenshot` subtype. Last, Go's `Asset` gains `Video` and `Screenshot`, and `internal/iphone/categories.go` adds `OldScreenshots` (screenshots older than a number of days, oldest first) and `LargestVideos` (the n largest videos counting sidecars, ties to the older one). Both leave out protected assets, and `OldScreenshots` leaves out undated ones.
- **Increment 7:** `FindDuplicates` (`internal/iphone/duplicates.go`) runs the pipeline on a `list`: `analyze` on the photos, `hash` on the `ExactCandidates` (skipped when there are none), `NearDuplicates` and `ExactDuplicates`, then merges groups that share an asset and picks keepers with `Keep`. It returns `Group{IDs, Keep, Exact}`, where `Exact` means every member has the same SHA-256, plus the assets `analyze` or `hash` failed on.
- **Increment 8:** the iCloud Photos question and `gk setup photos`. `config.SetPhotosSource` writes `photosSource` into the config file and keeps every other key, creating it `0600` in a `0700` folder; an unreadable file is left alone. The question (`internal/cli/setup.go`) is a Bubble Tea v2.1.0 model with Yes, No and Not sure; Not sure shows where to look on the phone and asks again, and esc, q or ctrl+c cancel without saving. It only runs when stdin and stdout are terminals; otherwise `gk setup photos` says to run it in one or set `photosSource` in the config, and exits 1. Asking it on the first `gk iphone` comes with the photo scan.
- **Increment 9:** `gk iphone` scans photos. `iphone.ScanPhotos` runs `list`, `FindDuplicates`, `OldScreenshots` and `LargestVideos`. Before the spinner starts, `gk iphone` takes `photosSource` from the config or, the first time and only in a terminal, asks the question and saves the answer (not with `--dry`). After reading the phone it runs the scan through `~/bin/photos-helper` (`photoRunner` in tests), records it as the "iPhone photos" scanner in Run details, adds a Photos line to the summary, and adds a Photos part to the report's iPhone section: one row per duplicate group (when it was taken, how many photos, exact or look-alike, what deleting the extras would free), the old screenshots' count, size and dates, and the 10 largest videos. `Asset.TotalBytes` (size with sidecars) is shared by the report and `LargestVideos`.
- **Increment 10:** the saved review selection (`internal/clean/photos.go`). `NewSelection` turns the IDs a review approves into a `Selection{Time, Source, Delete}`, each entry holding the ID, creation date, size and sidecar size from the scan. `SaveSelection` and `LoadSelection` keep it in `~/.local/state/groundskeeper/photo-review.json` (one review at a time; saving replaces it), and `LoadSelection` gives `ErrNoSelection` when there is none. `history.save` is exported as `SaveJSON` for the `0600` file in a `0700` folder.
- **Increment 11, the review screen.** First, `gk iphone` saves its photo results for the review as `~/.local/state/groundskeeper/photo-scan.json` (`history.SavePhotoScan`, with `LoadPhotoScan` giving `ErrNoPhotoScan` when there is none), replacing the previous one. `--dry` saves nothing, and a failed photo scan leaves the last one in place. The photo types gained lowercase JSON tags to match the other state files. Second, `Helper.Thumbnails` runs the helper's `thumbnail --out <dir>` and returns each JPEG's path; `call` now passes extra flags after `--source`. Third, the review screen without images (`internal/tui/review.go`): `Review` steps through the groups, starting from each group's keepers; the arrow keys move between photos (up and down suit the text list, left and right the side-by-side thumbnails to come; both work), k keeps a photo or makes it an extra, enter accepts the group, s skips it, p goes back, q saves the decisions so far, ctrl+c quits without saving, and accepting the last group saves. `Delete` returns the extras of accepted groups. Fourth, `gk review photos` (`internal/cli/review.go`, in a new Review group of the help): it loads the last photo scan, runs the screen when stdin and stdout are terminals (`reviewRunner` in tests), and when the review is saved builds the selection with `NewSelection` and saves it, printing how many photos and how much space and that nothing has been deleted. With no scan it says to run `gk iphone` first; with no duplicates it says so; ctrl+c saves nothing. Fifth, the review fetches each group's thumbnails in the background as the group opens (`tui.Viewer`, `internal/tui/viewer.go`), once per photo, and space opens them in Quick Look (`qlmanage -p`, run with `tea.ExecProcess` so the screen waits until the window closes). `gk review photos` supplies the viewer: `photos-helper thumbnail` into an `os.MkdirTemp` folder (`0700`) removed when the review ends. If thumbnails can't be fetched, the screen says why and the review works on without them. Sixth, inline thumbnails in Ghostty (`internal/tui/kitty.go`): each fetched JPEG is decoded and sent as PNG in the background under the next image ID (`a=T`, `U=1`, 30 by 20 cells, `q=2`) with `tea.Raw`, and the group is drawn as a row of placeholder cells with Keep or Extra and the cursor under each photo, above the text list. `gk review photos` sets `Viewer.Inline` when `TERM_PROGRAM` is `ghostty`. A window too narrow for the group asks to be widened, a thumbnail that won't decode is reported with a pointer to Quick Look, and the last frame drops the images, whose data `Review.Cleanup` then deletes (`a=d`, `d=I`) once the program has ended. Seventh, thumbnails shrink to fit the window: each image is sent once without a placement (`a=t`) and placed separately (`a=p`, `U=1`, placement 1) at a width of up to 30 cells, two apart, with rows two thirds of that; resizing or going back to a group places its images again only when their size changes. Below 12 columns each, the screen asks for a wider window.
- **Increment 12, `gk clean photos`.** `Helper.Delete` (`internal/iphone/photos.go`) runs `photos-helper delete` with a `DeleteRequest{ID, Bytes, Created}` per photo and returns the IDs deleted and the failures. `Selection.Requests` builds the requests from the saved review and `Selection.Without` drops the deleted ones. The command (`internal/cli/clean_photos.go`) loads the saved review, shows the count, size, review date and source, and stops there unless `--apply`. With `--apply` it confirms, deletes through the helper, saves the review minus what was deleted, and lists each photo it couldn't delete with the reason (no longer there, changed since the scan, or Photos refused). A helper error deletes nothing and leaves the review as it was. `thumbnailProblem` became `helperProblem`, as it now explains delete failures too.

### Decisions

- **A `hash` command instead of dropping SHA-256.** The plan confirms exact duplicates with SHA-256, and no helper command read the original files. The user chose adding `hash` over treating a zero feature-print distance as exact.
- **`requestReadData`, not `requestDownload`.** Both are in the ImageCaptureCore headers (macOS 10.15+); `requestDownload` writes the photo to a file on the Mac that would then have to be deleted, while `requestReadData` hands back bytes that go straight into SHA-256.
- **Library hashing never goes to iCloud,** keeping the no-network rule. An original that isn't on the Mac fails with `read`, and Go leaves it out of exact-duplicate groups.
- **`NearDistance` is 0.025, calibrated on the phone.** `analyze` on all 1,158 photos gave 1,799 pairs within 10 minutes or one burst, with distances spread evenly from 0 to past 1.5 and no natural gap. The user judged 39 pairs from thumbnails shown side by side in shuffled order without distances: 14 sampled across 0 to 0.8, the 10 closest on the phone, and all 15 between 0.02 and 0.048. Every pair at 0.0226 or less (12) was a duplicate; between 0.026 and 0.048 answers were mixed (5 duplicates, 9 not), unordered by distance; nothing above 0.048 was. 0.025 groups none of the non-duplicates and misses 5 of 17 duplicates; 0.048 would catch all 17 but group 6 of 12 non-duplicates. Precision won, because each group's extra photos are marked for deletion and device deletes are permanent.
- **Only same-shape assets are hashed.** On the device path `hash` reads each file in full over USB (about 32 MB/s), and a byte-identical copy always has the same size and dimensions. No two of the phone's 1,158 files share a size, so on this phone nothing is hashed at all.
- **The hash alone never groups.** A matching SHA-256 with a different size can only be a stale or wrong answer, so size and dimensions are part of the grouping key.
- **A missing size never matches.** PhotoKit's size key is undocumented and gives 0 when missing; without the guard, every asset missing it would be hashed and could group.
- **Aesthetics alone picks the best shot; sharpness and face quality are deferred.** The user was shown the phone's 12 groups with left and right shuffled and no scores, and answered "either" for all 12. Within each pair the aesthetics scores differed by 0.063 at most and 0.002 or less in two. With no wrong pick to correct, adding two new Vision measurements to the helper wasn't justified. The plan now says so.
- **A favorite isn't kept alongside an extra photo.** When a group holds protected assets, the best shot is picked among them, so a group with one favorite keeps just that favorite. The plan only said protected photos are always kept.
- **It's a constant, not a config option.** One calibration on one phone doesn't say which way another user would move it.
- **The helper's stdout is decoded even when it exits 1,** because that's where its error kind is. Its stderr is never read.
- **Renders fold into their original on the device path,** as the library path already counts `fullSizePhoto` in `sidecarBytes`. Photos shows the pair as one photo (the Screenshots album counts 52, not 54), listing the render separately let an original and its render group together with the original marked for deletion, and the render is the only sign over USB that a photo was edited, which now protects it. The `IMG_E` name is Apple's convention, not an API, so a render is only folded when its original matches on folder, number and date.
- **Protected screenshots and videos are never suggested,** the same rule `Keep` follows. On the phone this also keeps edited ones out until deleting an edited original over USB has been tested (see Open).
- **Videos aren't analyzed.** Near-duplicates are for photos, and comparing a video's poster frame could group a video with a still from it. Videos can still be exact duplicates by hash.
- **Exact and near groups that share an asset become one group,** so the review shows each photo once. `Exact` is only true when the whole merged group is byte-identical, since only then is deleting the extras safe without looking.
- **Bubble Tea v2.1.0** (`charm.land/bubbletea/v2`, released 2026-10-08), the newest release on the module proxy, as the plan names Bubble Tea for this question and the review. In v2, `View` returns a `tea.View` and keys arrive as `tea.KeyPressMsg`, checked against the module's source.
- **Rewriting the config reorders its keys.** `SetPhotosSource` goes through a JSON map so unknown keys survive, and Go writes map keys sorted; comments aren't possible in JSON, so nothing else is lost.
- **A failed photo scan doesn't fail `gk iphone`.** The phone's storage and apps were already read, so the report is still written, with a note saying why photos are missing and what to do (install the helper, unlock, Photos access, reconnect). The note is printed after the summary and shown in the report, and Run details records the kind. `Helper.call` now wraps the run error along with `ErrHelper`, so a missing helper is recognized as `fs.ErrNotExist`.
- **The report summarizes; the review is where photos are judged.** A first version listed every photo in each group by ID, date and size, and the user found that wasn't enough to tell photos apart. Thumbnails would put copies of photos into reports kept for six months, against the rule that reports hold paths, sizes and dates only, so each group is now one row and old screenshots are a count, size and date range. `gk review photos` will show the thumbnails.
- **Photo results aren't saved in the iPhone snapshot yet;** the review decides what it needs saved.
- **Photos are shown by ID, date and size only,** as the privacy rules allow; IDs are file names on the phone and local identifiers in the library, rendered through `html/template`.
- **The selection refuses what must never be deleted.** An ID missing from the scan, or a favorite, edited photo or photo in an album, makes `NewSelection` fail, so the always-kept rule holds whatever the review screen allows. Each entry keeps the size and date `photos-helper delete` checks, so a photo that changed since the review is refused at delete time too.
- **The selection lives beside the snapshots but isn't one.** Its name doesn't match the snapshot pattern, so `Prune` never removes it and the history readers skip it.
- **The review reads the last saved scan rather than rescanning.** A scan takes over 2 minutes on the phone, and the review should start at once. A photo that changed since the scan is still refused when deleting, by the size and date check in `photos-helper delete`. The file holds every asset (about 180 KB of JSON for 1,146 on the phone), since `Keep` and the selection need their sizes, dates and protection.
- **Inline thumbnails use Kitty graphics with Unicode placeholders (spike, outside the repo).** Each image is sent once with `tea.Raw` (`a=T`, `U=1`, columns and rows), and the view prints placeholder cells (`U+10EEEE` with row and column diacritics, the image ID as a 256-color foreground), so Bubble Tea's renderer handles them as ordinary text. `charmbracelet/x/ansi/kitty`, already a dependency through Bubble Tea, provides the encoder, `Placeholder` and `Diacritic`. In Ghostty 1.3.1 two 768x1024 thumbnails showed side by side with text above and below whether they were sent before the program started, from `Init`, or a second after the first frame. Ghostty's full-screen (alternate) screen wasn't tested, so the review draws inline. Other terminals (VS Code's, Terminal.app) need the Quick Look fallback; which terminals get inline images is decided by the review.
- **The screen enforces the keep rules, and the selection enforces them again.** A favorite, edited photo or photo in an album can't be made an extra, and the last kept photo of a group can't be dropped; each refusal shows a note. `NewSelection` refuses protected photos regardless, so the screen's check is for feedback, not safety.
- **A skipped group deletes nothing,** and so does a group left undecided by quitting early.
- **Quick Look works in every terminal, so it comes first;** inline images in Ghostty come next. The command builds the `qlmanage` process with `exec.CommandContext` and separate arguments and passes it in, so `internal/tui` runs no programs of its own and tests see the paths without opening a window. Its output is discarded, as `qlmanage` logs to stdout.
- **Inline images only in Ghostty,** the one terminal the spike tested. Ghostty sets `TERM_PROGRAM=ghostty` for every shell it starts (`src/termio/Exec.zig` in its source), with or without shell integration. Kitty supports the same protocol but wasn't tried. Space still opens Quick Look in Ghostty.
- **Sent as PNG because the protocol doesn't take JPEG;** it accepts RGB, RGBA and PNG (https://sw.kovidgoyal.net/kitty/graphics-protocol/). Kitty fits each image into the 30 by 20 cell box keeping its aspect ratio, so portrait and landscape thumbnails share one size, and a window resize doesn't resend anything.
- **At most 255 images per review, never reused.** IDs go in the 256-color foreground, which holds 8 bits, and 0 isn't an ID. Reusing an ID would replace an image that an earlier group still points to, so going back could show the wrong photo while deciding what to delete. Past 255 the screen says so and shows text only.
- **The images are deleted when the review ends,** so photo thumbnails don't stay in the terminal's memory; the last frame is drawn without them first, so nothing in the scrollback points at a deleted image.
- **Thumbnails are sized to the window, not fixed.** A 3-photo group in an 88-column window hid its images behind "Widen the window" (see Problems found). Sending the image data once and placing it separately lets a resize or a different group size place it again without resending about 2.5 MB per image. Each image keeps placement 1, so a new placement replaces the old one rather than adding a second that placeholders might pick instead.
- **On the iPhone, `--apply` asks for the number of photos to be typed** (the user's choice over y/N and over requiring an import to the Mac first). Device deletes are permanent and skip Recently Deleted, so a reflexive `y` deletes nothing. The prompt says it's permanent first.
- **In the Mac library, y/N is enough,** because macOS shows its own confirmation and the photos stay in Recently Deleted for 30 days.
- **The saved review keeps what wasn't deleted,** so a refused or failed photo can be retried or reviewed again, and a finished review reports nothing left to delete. A helper error deletes nothing, so the review is left unchanged.
- **Dates go to the helper as Go writes them.** Swift's `.iso8601` decoding strategy accepted `Z`, a `-07:00` offset and fractional seconds in a scratch program on this Mac, so no conversion is needed.
- **Screenshots are told apart by metadata, not by file type or size.** A read-only probe outside the repo found, on the phone, EXIF `UserComment` = `Screenshot` on 52 PNGs (24 at 1260x2736, 28 at 2736x1260) and on the 2 `IMG_E` renders of two of them; no other image had it, including an 800x533 PNG that a PNG rule would have caught. The phone's Screenshots album counts 52. In the Mac library, PhotoKit's `photoScreenshot` subtype was set on exactly the 4 assets in the Screenshots smart album, though the header marks it only for iOS. Videos are `public.movie` by UTI on the phone (19) and `mediaType == .video` in the library.

### Verified

- `photos_test.go` replays hand-written helper JSON in `testdata/helper/` (device and library `list`, `analyze`, a `locked` error) and checks the arguments and stdin sent. Mapping `locked` to the wrong kind fails `TestHelperErrorKind`. The fixture guard passes over the new files.
- `hash --source device` on the phone, for the 3 smallest files (PNG screenshots) and the largest (a 643 MB video): every hash equalled the SHA-256 of the same file read separately over go-ios AFC (`/DCIM/NNNAPPLE/<name>`, read-only) from a scratch program outside the repo, byte counts equal to `list`'s `bytes`. The video crosses 154 chunk boundaries. It took 20 s in all, about 32 MB/s. A made-up ID gave `not-found`; a missing `--source`, an extra `--out` and non-JSON stdin each exit 2. `TestHash` covers the Go side.
- `group_test.go` covers the window, bursts, chaining, and skipping unanalyzed and undated photos. Removing the burst linking fails `TestNearDuplicatesBurst`, and dropping the window limit fails `TestNearDuplicatesWindow`.
- `NearDuplicates` on the phone's real `list` and `analyze` output (from a temporary test, not committed) gave 12 groups of 2, exactly the 12 pairs at 0.025 or less, all of which the user had judged duplicates.
- With the phone unplugged, `list --source device` answered `{"error":"no-device"}` with exit 1, which closes that part of step 10's open item.
- `exact_test.go` covers matching, same shape with different contents, a failed hash, missing sizes, and a shared hash with different sizes. Removing the zero-size guard fails `TestExactCandidates`; grouping by hash alone fails `TestExactDuplicatesNeedsSameShape`.
- In the Mac library the user imported a screenshot twice. `list` gave 6 assets; the same-shape filter picked the 2 copies, `hash` gave both the same SHA-256 with no failures, and the Go `ExactCandidates` and `ExactDuplicates`, run on the saved output from a temporary test, picked the same 2 and returned one group of exactly those two.
- `best_test.go` covers the best score, protected assets beating a higher score, ties and missing scores. Letting the best shot ignore protection fails `TestKeepProtected` and `TestKeepEditedOverBetterScore`.
- Device `list` after folding renders: 1,159 assets became 1,146; the 13 removed were exactly the `IMG_E` files, each with one original sharing folder, number and date; the 13 originals are `edited` with `sidecarBytes` equal to the old value plus the render's bytes; the other 1,133 assets were unchanged.
- Device `list` with the new flags: 52 screenshots (all PNG, 28 at 2736x1260, 24 at 1260x2736), equal to the Screenshots album, and 19 videos (18 MOV, 1 MP4), none both; the same 1,146 IDs with every other field unchanged; `list` still took 0.44 s. Library `list`, run from Terminal.app: the 4 PNGs in the Screenshots smart album are `screenshot` and the 2 JPEGs aren't. The library has no videos, so its `video` flag is untested.
- `categories_test.go` covers age, protection, undated screenshots, sidecar sizes and ties; dropping each of those five rules fails a test. On the phone's real `list` output (a temporary test, not committed), `OldScreenshots` with 30 days gave 48 of the 52 screenshots (the 2 edited ones left out) and `LargestVideos` gave 643.0, 588.6, 542.8, 517.3 and 350.7 MB, both equal to an independent count from the same JSON.
- `duplicates_test.go` drives `FindDuplicates` through a scripted helper. Seven mutations each fail a test: analyzing videos, hashing with no candidates, not merging, ignoring exact groups, `Exact` always true, `Keep` without scores, and dropping failures.
- `FindDuplicates` against the real helper on the phone (a temporary test, not committed): `list` gave 1,146 assets in 0.34 s, and the pipeline took 2 min 18 s with no failures. It found 6 groups of 2, all near-duplicates taken within a second, keeping one each, with no render and no edited photo in any group. This closes the open item about decoding the real helper's output.
- `config_test.go` covers keeping other keys, creating a private file and leaving an invalid one; dropping the existing values, the `Chmod`, the `0700` folder or the invalid-JSON error each fails a test. `setup_test.go` drives the question through `Update` (yes, no, the ends of the list, Not sure asking again, cancel) and once through a real `tea.Program` reading `j` and `\r` from a reader; the command tests cover saving, cancel changing nothing, and the no-terminal error, which the real asker gives under `go test`. Saving on cancel, never saving, Not sure quitting and losing the config path in the error each fail a test. The built binary with stdin from `/dev/null` printed the no-terminal error and exited 1. In Terminal.app with a scratch `--config`, the user chose Not sure (the where-to-check text appeared and the question stayed), moved with the arrow keys and chose No: it printed `Saved photosSource "device"` and the new file held only that key.
- `photoscan_test.go` covers `ScanPhotos` and a `list` error. `iphone_photos_test.go` runs `gk iphone` against the phone fixture with a fake helper: the scan with a saved source, asking once (not saved by `--dry`, then saved and not asked again), skipping without a terminal, a missing helper (the real runner against the fake home), a locked phone, the report's Photos part, the note in the report, and a `<script>` photo ID escaped. Eleven mutations each fail a test: saving on `--dry`, never saving, the missing-helper kind and note, the locked note, skipping the scan, not printing the note, dropping the Run details entry, counting keepers as freed, inverting Keep (both seen in the freed size), and leaving sidecars out of screenshot sizes.
- `gk iphone --dry` on the real phone and Mac: the summary showed `Photos       6 duplicate groups, 48 old screenshots`, matching increments 6 and 7, and the run took 3 min 12 s in all.
- `internal/clean/photos_test.go` covers building a selection, refusing protected and unknown IDs, a save and load round trip with `0600` and `0700`, `ErrNoSelection` before saving, and surviving `Prune`. Allowing protected photos, allowing unknown IDs, leaving sidecars out of the size, giving the file a snapshot-like name, and returning the raw not-found error each fail a test.
- Saving the photo scan: `TestIPhoneScansPhotos` loads it back, `TestIPhoneDrySavesNoPhotoScan` and `TestIPhoneFailedPhotoScanKeepsLastOne` cover `--dry` and a failed scan, and `TestPhotoScanSurvivesPruning` checks pruning leaves it and the history readers skip it. Never saving, the untyped not-found error and a snapshot-like name each fail a test.
- `TestThumbnails` replays a hand-written `thumbnail` answer and checks the `--out` flag; dropping the extra flags fails it. On the phone (a temporary test, not committed), two photos gave 768x1024 JPEGs in 0.32 s, `0600` in an `os.MkdirTemp` folder that was `0700`, and a made-up ID failed with `not-found`.
- `review_test.go` drives the screen with key presses: moving with left and right or up and down, accepting every group, skipping, changing keepers, a protected photo, the last keeper, q saving decisions so far, ctrl+c not saving, going back to change a decision, and the view. Dropping the protected check, dropping the last-keeper check, deleting from skipped groups, saving on ctrl+c, not saving after the last group, and ignoring keepers each fail a test.
- `internal/cli/review_test.go` runs `gk review photos` with a key-pressing runner: saving a selection with the scan's source, ctrl+c saving nothing, no scan, no duplicates, no terminal (the real runner under `go test`), and its place in the Review group. Saving on cancel, the wrong source, never saving, and losing the run-gk-iphone hint each fail a test.
- `viewer_test.go` covers fetching each group once (going back doesn't refetch), Quick Look getting the current group's thumbnails, and a failed fetch: the reason shows, space says there's nothing to show, and the review still saves. Refetching every time, not fetching at the start or on the next group, hiding the error, and previewing another group's photos each fail a test. `TestReviewPhotosFetchesThumbnailsAndRemovesThem` runs the command with a fake helper and checks the thumbnails were asked for and their folder is gone afterwards; without `os.RemoveAll` it fails. That mutation run left its test folders in the temp directory, which were removed by path. In Ghostty with the phone connected, the user pressed space on a group and Quick Look opened its photos; closing it returned to the review.
- `kitty_test.go`, `inline_test.go` and `TestReviewPhotosInlineOnlyInGhostty` cover the image command, the placeholder cells, showing each group's images and moving on, no images without `Inline`, a narrow window, a thumbnail that won't decode, and quitting. Dropping `Transmission: kitty.Direct`, the image row's quit check, the `Inline` check, the width check, or the `TERM_PROGRAM` check each fail a test. In Ghostty with the phone connected, the user saw each group's photos side by side with Keep and Extra under them, the cursor moved under the images, the next group's photos replaced the first's, and quitting left clean text.
- `TestThumbSize`, `TestPlaceImage`, `TestReviewInlineShrinksToFit` and `TestReviewInlineGoingBackPlacesAgain` cover sizing, placing on arrival, on resize and on going back, and not placing again at an unchanged size. Dropping the placement on arrival, on resize or on going back, or placing every time, each fail a test. In Ghostty at about 88 columns, the user saw all three photos of a 3-photo group.
- `TestDelete` checks the requests sent and the answer decoded. `clean_photos_test.go` covers no saved review, the dry run, `y`, `yes`, a wrong number and an empty answer on the device (none deletes), the typed count, y/N for the library, the review keeping the photo that wasn't deleted, and a locked phone leaving the review unchanged. Accepting `y` on the device, not updating the review, skipping the dry-run check, and hiding a helper error each fail a test.
- On the phone, with three throwaway near-identical shots: `gk review photos` showed them as one group, the user skipped the other six groups and accepted that one, and `gk clean photos --apply` asked for `2`. It deleted 2 photos with no failures in 0.3 s, freeing 4,897,423 bytes, and `gk clean photos` then said the saved review had nothing left to delete. A fresh `list --source device` showed one photo left from that minute, the one marked Keep, with the same size.

### Problems found

- **The spike's first images were blank: `kitty.EncodeGraphics` sent no image data.** The user saw text and empty space in Ghostty. Capturing the spike's output to a file and through a pseudo-terminal (`script`) showed the placeholders and their colors intact, but each image command was a 36-byte header with no payload; calling the encoder alone gave the same 36 bytes and no error. Cause: it only encodes the image inside `switch o.Transmission`, whose zero value matches no case, so `Transmission: kitty.Direct` must be set even though the protocol defaults to direct. With it, each thumbnail is about 2.5 MB of PNG chunks. A second blank run followed; that version deleted its images on quit and the later ones didn't, so the screenshot was most likely taken after quitting, but it also differed in image IDs and quiet mode, so that isn't proven. A plain program without Bubble Tea then showed the photo both placed directly and through placeholders, with no error replies, and the three timing variants all worked.
- **`gk review photos` was an unknown command, and a new `gk iphone` didn't pick up new photos.** `~/bin/groundskeeper` was installed from a commit before photo scanning and the review were added. Its `gk iphone` logged no "iPhone photos" scanner and took 43 s (the photo scan alone takes over 2 minutes), and `photo-scan.json` still held the previous scan. `make install` fixed both. Reinstall after each increment that's tried as `gk`.
- **A 3-photo group showed "Widen the window" instead of photos.** The user's screenshot showed the message in a window about 88 columns wide; three fixed 30-column thumbnails with gaps need 96. The rule worked as written; the fixed size was the flaw, fixed by sizing to the window (see Decisions).
- **Half of increment 3's near-duplicate groups were a photo and its own edit.** The pipeline found 6 groups where increment 3 found 12. Analyzing the 13 `IMG_E` renders and their originals (a temporary test) put 6 pairs at 0.0016 to 0.0226, under `NearDistance`, and each render has its original's timestamp, so before increment 6 each formed a group: 12 − 6 = 6. Increment 3's IDs weren't saved, so this rests on the counts matching. With renders listed as their own assets, `Keep` could have kept the render and deleted the original; increment 6's folding removed that.
- **Library `list` counted every original twice.** After increment 2, the two unedited library JPEGs had `sidecarBytes` equal to `bytes`. A read-only probe outside the repo listed each asset's PhotoKit resources: those two had only their `photo` resource, and the edited screenshot's `photo`, `adjustmentData` and `fullSizePhoto` came to 32,522 + 2,048 + 12,467 = 47,037 bytes, exactly the `sidecarBytes` `list` gave, original included. Cause: increment 2 moved the original's lookup into `originalResource(of:)`, which called `assetResources(for:)` again, and that returns new objects, so `Asset`'s identity check (`!==`) never matched the original. Fixed by having `originalResource(in:)` pick from the array the caller already holds. Afterwards `sidecarBytes` was 0, 0, 14,515 and 0, matching the probe, and the three hashes were unchanged.

### Open

- `hash --source mac-library` ran from a terminal app with Photos access and hashed the 3 oldest library photos (two JPEGs and an edited PNG screenshot) with no failures; from the agent's terminal it answers `denied`. Each hash equalled `shasum -a 256` of the photo's file in the library's `originals` folder (named by the ID's UUID), and each file's size equalled `list`'s `bytes`. The edited screenshot hashed its unedited original, as intended.
- An edited library photo's render is reported, as PhotoKit's `fullSizePhoto` resource (12,467 bytes on the edited screenshot), so it's counted in `sidecarBytes`. This answers step 10's open question.
- Vision's ranking doesn't always match the user's eye: of the first 14 pairs, the user found the one at 0.075 more alike than the one at 0.048.
- Every pair the user called a duplicate was taken within 2 seconds, and every pair 12 s or more apart wasn't. A time rule might separate the mixed 0.026 to 0.048 range, but that's 39 pairs on one phone, so the 10-minute window stays.
- The best-shot check couldn't test a sharp shot against a blurry one: at the 0.025 cutoff the grouped photos are nearly identical. If reviews show bad picks, sharpness (not a Vision request; would need its own measure) and Vision's face capture quality are the candidates.
- Both copies of the library's exact pair report `inAlbum: true`, so `Keep` keeps both and the group has nothing to delete. The user only dragged them into the library, but a read-only probe (`fetchAssetCollectionsContaining`, album subtypes only) found every screenshot in the library, both copies included, in a `regular` album, the kind a person makes, and none in an imported or synced one. So the flag and the rule are right; how the copies got into that album (perhaps it was open in the sidebar during the drop) isn't known.
- Exact duplicates on the phone itself weren't tested, since it has none. Whether Photos' Duplicate command on the phone makes a byte-identical copy isn't known.
- Over AFC the camera roll's folders are `100APPLE` to `122APPLE`, while ImageCaptureCore names them `YYYYMM_a`, so AFC paths and helper IDs share only the file name.
- Whether deleting an edited original on the device path also removes its render is untested. Edited photos are protected, so nothing suggests deleting one; test with a throwaway edited photo before that changes.
- `clean photos` on the Mac library hasn't run against a real library, so macOS's confirmation and Recently Deleted are still unverified there.
- Whether space opens Quick Look in the current build wasn't confirmed again after the inline images were added.
- Inline images in Kitty, inside tmux, and in Ghostty's alternate screen are untested; tmux changes `TERM_PROGRAM`, so it gets Quick Look only.
- How many largest videos to show isn't decided; `LargestVideos` takes n, and the review or report will pick it.
- **`NearDistance` was calibrated partly on render pairs.** The renders' distances to their originals (0.0016 to 0.6345, with 0.0404, 0.0415 and 0.0752 among them) overlap the calibration range, so some of the 39 pairs the user judged in increment 3, and some of the 12 best-shot comparisons in increment 5, were likely a photo against its own edit. The judged pairs weren't saved, so which ones can't be checked. With renders folded, a fresh calibration on true pairs could move the cutoff; until then 0.025 stays, since it still grouped nothing the user called different.

## Step 10: photos-helper

### Built

- **Increment 1:** the Swift package `helpers/photos-helper` (Swift 6 language mode, macOS 26+) with `list --source device`. It opens the first iPhone on USB with ImageCaptureCore and writes `{"source","assets":[{id, created, width, height, bytes, sidecarBytes, burst}]}` to stdout. Errors are `{"error": "no-device" | "locked" | "timeout"}` with exit 1; bad arguments print usage and exit 2. `make build` also builds it into `bin/photos-helper`.
- **Increment 2:** `analyze --source device` reads a JSON array of asset IDs on stdin, fetches a 512-pixel thumbnail of each from the phone, and runs Vision's `GenerateImageFeaturePrintRequest` and `CalculateImageAestheticsScoresRequest` (the Swift API, macOS 15+). It writes `{"source","analyses":[{id, featurePrint, aesthetics, utility}],"failed":[{id, reason}]}`, where reason is `not-found`, `thumbnail` or `vision`.
- **Increment 3:** `thumbnail --source device --out <dir>` reads asset IDs on stdin and saves the phone's 1024-pixel JPEG thumbnail of each, without re-encoding. It writes `{"source","thumbnails":[{id, path}],"failed":[{id, reason}]}`; `write` joins the failure reasons. Flags are parsed as `--name value` pairs, and unknown or missing flags exit 2.
- **Increment 5:** every command takes `--source mac-library` (`Library.swift`). `list` adds `favorite`, `edited` and `inAlbum`, which the device path leaves out; the ID is PhotoKit's local identifier. `bytes` is the original's size from `PHAssetResource`'s `fileSize` key, and everything else (Live Photo video, edits, renders) is `sidecarBytes`. Without Photos access every command answers `{"error": "denied"}` with exit 1. `analyze` and `thumbnail` share their loop with the device path, which now only supplies the lookup and the image fetch. `delete` refuses mismatches as on the device, deletes in one `performChanges`, and reports as deleted only the IDs that can't be fetched afterwards.
- **Increment 6 (before 5):** `make install` copies `bin/photos-helper` to `~/bin/photos-helper` (`0755`) and `make uninstall` removes it. `doctor.HelperPath` names that path for the Go side to run, and `gk doctor` has a "Photos helper" check: missing, or not an executable file, is a warning with "Run make install" as the fix.
- **Increment 7:** `photos-helper access` prints `{"access": "authorized" | "limited" | "denied" | "restricted" | "not-determined"}` from `PHPhotoLibrary.authorizationStatus(for: .readWrite)`, without prompting, and takes no `--source`. `gk doctor` has a "Photos access" check after "Photos helper": `authorized` is ok, every other answer is a warning with the System Settings fix, and a helper that can't answer gives "couldn't check" with "Run make install".
- **Increment 4:** `delete --source device` reads `[{id, bytes, created}]` on stdin, refuses any asset whose size differs or whose date is more than a second off (`mismatch`), and deletes the rest in one `requestDeleteFiles` call. It writes `{"source","deleted":[ids],"failed":[{id, reason}]}`, with `delete` for files the phone didn't report as deleted.

### Decisions

- **Library images come from the image data, not `requestImage`.** See Problems found. `requestImageDataAndOrientation` returns the current version, so analysis and review show the photo with its edits.
- **Library thumbnails are re-encoded from pixels only** (JPEG, quality 0.8), so no metadata is copied. ImageIO adds just pixel dimensions and color space to the Exif block.
- **Hidden photos are left out of the library listing,** so a review never shows a photo the user hid. All burst photos are included, as on the device path.
- **The library delete keeps the size and date check.** Local identifiers aren't reused the way phone file names are, but one input format and one rule for both sources is simpler, and it still catches a stale selection.
- **The helper lives in `~/bin`, next to `groundskeeper`.** The plan only said "the install location"; one folder keeps install, uninstall and Full Disk Access advice in one place. It's run by absolute path, so being on `$PATH` doesn't matter.
- **`access` reads the status; it never requests it.** `doctor` only reads, and Apple's page for `authorizationStatus(for:)` lists the statuses without saying whether it prompts, so this was checked by running it (see Verified). Like Full Disk Access, the answer belongs to the terminal app `doctor` runs from.
- **Anything short of full access is a warning.** `limited` would leave most of the library out of a review, and a missing Photos grant doesn't affect the Mac scan or the schedule.
- **A missing helper is a warning, not a failure.** The Mac scan and the schedule don't use it, and a failed check makes `doctor` exit with an error.
- **The check doesn't run the helper.** Stat and the execute bit are enough to catch a missing or half-finished install; running it would add a process for little gain.
- **The asset ID is the folder and file name on the phone.** A probe over 1,162 files on two sessions found `originatingAssetID` and `fingerprint` empty for every file and `ptpObjectHandle` the same for every file. Folder and file name was unique for every file and identical across both sessions (same SHA-256 of the sorted list). `delete` should still check size and date before deleting, as the step 8 spike did.
- **No phone is detected with a 10 s timeout**, not `deviceBrowserDidEnumerateLocalDevices`: that callback fired with zero devices before the phone was added, on every run.
- **Open errors aren't final.** With the phone unlocked, each run still got "access restricted", an open error -9943, then "access restriction removed", and the catalog arrived without retrying. So `list` waits up to 120 s for the catalog and reports `locked` only if the phone is still restricted then.
- **Feature-print distance is squared Euclidean.** Plain Euclidean distance over the raw floats differed from Vision's `distance(to:)` by up to 0.25 on 19 pairs of real photos; its square differed by at most 6.6e-8. So `featurePrint` is the raw 768 float32 values, and step 11's Go grouping computes the same distance.
- **512-pixel thumbnails.** On 20 photos, 512 and 1024 pixels both took about 130 ms per photo to fetch (most of it USB round trips) and Vision under 20 ms, with near-identical scores (aesthetics 0.27 against 0.24 on one photo).
- **Thumbnail files are named by the first 16 bytes of the ID's SHA-256.** IDs come from the phone, so they're never used as paths. The directory is created `0700` and the files `0600`: they are the user's photos.
- **1024-pixel review thumbnails.** Fetching took the same time as 512 in increment 2, and Quick Look needs enough detail to tell near-duplicates apart.
- **The camera roll over USB is a snapshot taken at connection.** A photo taken while the phone was plugged in didn't appear in two listings two minutes later; after unplugging and reconnecting it was there (1,158 to 1,159 assets). Step 11 should tell the user to reconnect if the newest photos are missing.
- No GPS (`gpsString`), file contents or device name in the output. `sidecarBytes` counts Live Photo videos and edit files (192 sidecars on the test phone), so space estimates aren't short.

### Verified

- On the phone (iOS 27.0.1): 1,162 assets, all with unique IDs, a date and dimensions; no GPS or location keys in the output. Folders are named `YYYYMM__`, not `100APPLE`.
- Bad arguments exit 2 with usage.
- `thumbnail` for the 5 newest photos plus a made-up ID, into a new nested directory: 5 JPEGs at 768x1024 or 1018x764, both directories `drwx------`, files `-rw-------`, names 32 hex characters, the made-up ID `not-found`. ImageIO found no GPS dictionary in any of them. Into a read-only directory, every photo failed with `write`. A missing `--out`, `--out` on `list`, an unknown flag and `--source mac-library` each exit 2.
- `delete`, on two throwaway photos the user took for the test, each the only asset from the last hour: a size one byte off and a date five seconds off were each refused as `mismatch` and the photo stayed. The exact request deleted it: the second run printed it as deleted with exit 0, and the next listing no longer had it (1,159 to 1,158). An unknown ID gave `not-found`; input without `created` exited 2.
- Increment 5, from Ghostty with Photos access, on 2 existing photos and 3 throwaway screenshots: `list` gave all 5, and favorite, edited and in-album matched what was set in Photos. The `bytes` of each screenshot equalled its source file's size exactly. No location keys. From VS Code each command answered `denied` with exit 1.
- `analyze --source mac-library`: 5 prints of 3,072 bytes; the two older photos, taken together, were 0.14 apart and every other pair 0.79 to 1.23. `thumbnail`: 5 JPEGs at most 1,024 pixels (the 758x116 one not scaled up), `0700` and `0600`, Exif holding only dimensions and color space. A made-up ID gave `not-found` in each.
- `delete --source mac-library`: a size one byte off and a date five seconds off were each `mismatch`, with no dialog. The exact request showed macOS's confirmation; after Delete, the helper reported it deleted, and the user found it in Recently Deleted.
- Device path after the refactor: `list` 1,158 assets with no new keys, `analyze` and `thumbnail` on the 5 newest unchanged in shape and size, and a size-mismatched `delete` refused with nothing deleted.
- Increment 6: `TestPhotosHelper` covers missing, not executable and installed; it fails with the execute-bit check removed. `make test` passes. After `make install`, `~/bin/photos-helper` is `-rwxr-xr-x`, runs and prints usage (exit 2), and `gk doctor` shows "ok Photos helper" and exits 0.
- Increment 7: `access` printed `authorized` from Ghostty and `not-determined` from VS Code, each with exit 0 and no dialog; an extra argument exits 2. `TestPhotosAccess` covers the five statuses and an unusable answer, and fails when `limited` is made ok. After `make install`, `gk doctor` from Ghostty shows "ok Photos access". Before reinstalling, the line was missing because `~/bin` still held the older build.
- `analyze` on all 1,162 assets plus one made-up ID: 140 s, 1,162 analyses, every print 3,072 bytes, the made-up ID reported as `not-found`.

### Problems found

- **Library `analyze` failed every photo with `thumbnail`.** Temporary stderr diagnostics showed PhotoKit's `requestImage` calling back once per photo with no image, no error, and not degraded, in iCloud or cancelled. A probe on one photo found `requestImage` gave nothing with opportunistic delivery, high quality, or full size, while `requestImageDataAndOrientation` and `PHAssetResourceManager.requestData` both returned 260,189 bytes, exactly the size `list` reported. tccd allowed every Photos check; its "Failed to issue generic sandbox extension" error also appeared in the `list` run that worked, so it isn't the cause. Why `requestImage` fails in a terminal process isn't known. Fixed by decoding the image data with ImageIO.
- **Library `delete` crashed with a trace trap** (exit 133) before deleting. The crash report showed `_dispatch_assert_queue_fail` under `_swift_task_checkIsolatedSwift` in "closure #1 in delete(_:inLibrary:)" on `com.apple.PHPhotoLibrary.transaction.high`: the `performChanges` block was inferred `@MainActor`, the same cause as the device crash below. Fixed by marking it `@Sendable`; the next run deleted the photo with no new crash report.
- **`delete` crashed after the phone deleted the photo.** The first real delete removed the photo but the helper died with no output. The crash report showed `EXC_BREAKPOINT` in `_dispatch_assert_queue_fail` under `_swift_task_checkIsolatedSwift`, inside the completion block of `requestDeleteFiles`, on `com.apple.root.user-initiated-qos`. The block was written inside a `@MainActor` function, so Swift 6 inferred it as main-actor isolated, and ImageCaptureCore called it on a background queue. Fixed by marking the ImageCaptureCore blocks `@Sendable`, which makes them nonisolated; they only resume a continuation. The thumbnail block got the same fix, since its header says it may run on any queue, though it hadn't crashed in 1,162 calls. The second throwaway deleted cleanly with no new crash report.
- **The catalog timeout would have cut off `analyze`.** In increment 1, the 120 s timer kept running after the catalog arrived, and its handler fails with `locked` or `timeout`; a full `analyze` takes 140 s. Found while reading the code for increment 2. Fixed by stopping the timers once the catalog is delivered; the full 140 s run now finishes.

### Open

- VS Code reports `not-determined`, yet `requestAuthorization` from VS Code answers `denied` without a prompt, so `doctor` there says "not granted yet" when macOS won't ask. The fix text covers both.
- The `no-device` and `locked` paths still need a run with the phone unplugged, and with it locked.
- Favorites, albums and edits aren't visible over USB. Edits may be detectable from `.AAE` sidecars.
- Whether file names stay the same after other photos are deleted.
- `utility` isn't a screenshot detector: it flagged 26 of 53 PNGs (iPhone screenshots are PNG) and 249 of 1,077 HEIC photos. Step 11 needs another way to find screenshots.
- Videos are analyzed from their poster frame.
- 4 assets disappeared from the listing between the morning's run and the delete tests, before any delete had run. Not yet explained; the user may have deleted them on the phone.
- Whether deleting a Live Photo also removes its video sidecar. Both test photos had no sidecars.
- `thumbnail` doesn't tighten the permissions of an `--out` directory that already exists, and doesn't delete old thumbnails. Step 11's Go side creates and cleans that directory.
- Sharpness and face quality for best-shot scoring aren't computed yet.
- Library sizes were only checked on screenshots. The edited screenshot's `sidecarBytes` was 2,048, probably the adjustment data alone; whether the edited render's size is reported isn't known. Live Photos, videos and photos whose original is only in iCloud weren't in the test library.
- Library videos fail `analyze` and `thumbnail`: the image data path has no poster frame for them.

## Step 9: iPhone storage, apps and shared files

### Built

- **Reader and recorder (`internal/iphone`, `internal/iphone/record`):**
  - `Read` gets lockdown values, `com.apple.disk_usage`, user apps from its own installation_proxy `Browse` with `ReturnAttributes` (go-ios's `BrowseUserApps` returns no sizes), and the files under each file-sharing app's `/Documents`.
  - go-ios's logger is set to discard, and errors are reported by kind (`ErrNoDevice`, `ErrNotTrusted`, `ErrRead`, `ErrUninstall`).
  - `go run ./internal/iphone/record` saves a read through `fixtures.Scrub` to `testdata/phone.json`.
- **`Parse`:** model, iOS version, space, apps largest first, and shared containers with their five largest files.
- **`gk iphone`:** reads the phone first, so a missing or untrusted phone fails before the Mac scan. Then it scans the Mac and writes the report with an iPhone section. iPhone snapshots go to the state directory's `iphone` folder (`0600`, six-month pruning). `--dry` saves nothing.
- **`gk clean iphone-apps <bundleID>...`:** only IDs from the last iPhone snapshot. Refuses Apple's apps and `iphoneKeep`. Dry run unless `--apply`; with it, asks per app, warns the data is lost for good, and suggests offloading. Only `y` or `yes` uninstalls. IDs complete from the snapshot.
- **Probably unused flag:** `history.Unused` flags apps whose data size stayed exactly the same for `iphoneUnusedDays`, in scans no more than 30 days apart. The report lists them in their own table with a shell-quoted hint and the number of scans the list is based on.
- **Spinner:** `gk scan` and `gk iphone` show the current step on stderr ("Reading iPhone", "Scanning <scanner>", "Writing report").

### Decisions

- Uninstalls are logged but not saved as clean snapshots, which count space freed on the Mac.
- The unused flag needs scans at most 30 days apart, because the launchd job doesn't read the phone, and two scans months apart can't show an app went unused in between.
- The spinner is a small ticker in `internal/ui`, not Bubble Tea, which would put the terminal in raw mode and take over Ctrl-C for one status line. It is silent unless stdout and stderr are both terminals, and with `--verbose`.

### Problems found

- **No progress shown during a scan.** The user saw only a blinking cursor during `gk iphone`. Root cause: the spinner was deferred in step 1 and never built; there was no code that wrote progress at all.
- **The unused flag flagged apps whose size had just changed.** Found while writing `Unused`: when the newest snapshot didn't match, the run's start stayed at zero time, which counts as older than any cutoff. A test with an app that changed since the last scan failed with the bug put back.
- **Sparse history would give confident flags.** One scan now and one in four months would flag every app with the same size in both. Fixed by the 30-day gap rule; a test with a 40-day gap fails without it.
- **go-ios brought in `golang.org/x/net` v0.57.0**, which govulncheck flagged (GO-2026-6617, 6612, 6611, 6603). Upgraded to v0.61.0.

### Verified

- `go test -race ./...` and `make test` (with govulncheck) pass.
- Mutation checks: removing the `iphoneKeep` check, the zero-time check and the 30-day gap each made a test fail.
- Spinner under a pseudo-terminal (`script`): frames animate and the label follows each scanner. Piped into `cat`, no spinner lines are written.
- On the real phone (iOS 27.0.1), with a throwaway App Store app installed for the test:
  - The dry run listed it (172.7 MB) and changed nothing.
  - `--apply` showed the app and data sizes and the warning, and `y` uninstalled it in under a second, with no prompt on the phone.
  - On a second app, answering `n` and pressing Enter alone each kept it (`kept, not confirmed`, then "Nothing was uninstalled.").
  - `com.apple.Pages` was refused without a prompt.
  - The next `gk iphone` snapshot no longer had the app.
  - `gk logs --cmd clean` showed each action with bundle IDs, bytes and outcomes, and no device name or UDID.

### Open

- Whether apps you never open keep exactly the same data size. If iOS changes it in the background, they are never flagged and the rule needs a tolerance. A few apps' sizes changed between scans minutes apart. Check after a few weeks of scans.
- iPhone history only grows when `gk iphone` is run by hand, because the phone usually isn't connected at the scheduled time.
- Why one app's data size differs from iPhone Storage by 18% (from step 8).

## Step 8: iPhone spike

### Built

- **`internal/fixtures`:**
  - `Check` flags UDIDs (both formats), MAC addresses, emails and bundle IDs outside `com.apple.*` and `com.example.*` anywhere in a file. Phone numbers, IMEIs and MEIDs are only checked under keys that name them, and must be the fixed fakes.
  - A guard test runs `Check` over every `testdata/` directory.
  - `Scrub` keeps only known keys, so fields a new iOS adds are dropped. It replaces device identifiers with fakes, maps third-party bundle IDs to `com.example.appN` (the same ID across one recording), renames third-party apps and files, and refuses to return anything `Check` still flags.
- Spike programs were throwaway and stayed in a scratch directory. Raw device responses were saved there with mode `0600` and never read into the repo.

### Answers (go-ios v1.3.2, iOS 27.0.1)

1. **App sizes.**
   - go-ios's `BrowseUserApps` sends no `ReturnAttributes` and returned no size fields for any of 160 apps. A `Browse` request of our own asking for `StaticDiskUsage` and `DynamicDiskUsage` returned both for every app.
   - Compared with Settings › iPhone Storage for two apps: the static size matched exactly (488.0 and 477.9 MB).
   - The data size of one app read 21.86 GB, then 8.77 GB about 20 minutes later, against 8.68 GB in Settings. The user had deleted two large downloads in that app in between, and only 2 of 154 apps changed by more than 10%, so the value is live, not a cache.
   - The other app read 1.43 GB against 1.17 GB in Settings, unchanged across readings. Unexplained.
   - Disk: `TotalDiskCapacity` 256,000,000,000 and `AmountDataAvailable` from `com.apple.disk_usage`; AFC's `FSFreeBytes` was within 0.4%.
2. **Uninstall.** `installationproxy.Uninstall` took about 0.4 s (`RemovingApplication`, `GeneratingApplicationMap`). The phone showed no prompt; the icon disappeared. The app was gone from the next app list, and free space grew by 2,092,937,216 bytes against the app's reported 1,996,226,560 (about 5% more, from one reading).
3. **Offload.** Not possible over USB. go-ios has no such command. libimobiledevice's `instproxy_archive` (zip the app on the device, then uninstall) is not the same as Offload anyway, and iOS 27 answers both `Archive` (sent for a bundle ID that doesn't exist) and the read-only `LookupArchives` with `UnknownCommand`. Offloading stays a report-only tip.
4. **PhotoKit (Mac library).**
   - The first run, from a terminal inside VS Code, got `.denied` with no prompt. The TCC log showed why: the responsible process was VS Code, which uses the hardened runtime without `com.apple.security.personal-information.photos-library`, so "Policy disallows prompt".
   - From Terminal.app, the prompt appeared and access was granted. `list` worked, and `PHAssetResource`'s undocumented `fileSize` key returned sizes.
   - The user's phone doesn't use iCloud Photos, so the Mac library holds almost nothing of theirs. `analyze` moves to step 10. `delete` wasn't tested, because a test image would upload to iCloud for a path this user doesn't use.
5. **ImageCaptureCore (iPhone over USB).**
   - Works from any terminal, with no Photos access involved. The phone must be unlocked: opening a session on a locked phone fails with code -9943 until the "access restriction removed" callback.
   - 1,162 files were listed in under a second, with size, dimensions, date and burst ID, and the phone reports `cameraDeviceCanDeleteOneFile`.
   - A throwaway photo, matched by exact size and timestamp, was deleted (`ICDeleteSuccessful`, 114 ms).
   - **The delete is permanent.** It was gone from the library and not in Recently Deleted or Recovered, while a second throwaway deleted on the phone itself did appear in Recently Deleted.
6. **Tunnel.** Not needed. Device info, disk space, the app list, uninstall and shared files all worked over plain USB with no tunnel running. go-ios's CLI only looks up a tunnel for developer services (`needsAutomaticTunnelInfo` in `cli_device_resolution.go`). `make install-tunnel` and the root LaunchDaemon are dropped.

### Problems found

- **Shared files all looked empty.**
  - Root cause: house_arrest falls back to documents-only access for App Store apps, listing `/` is then refused, and go-ios's `WalkDir` silently skips permission-denied errors. Listing `/Documents` returned entries, and walking it gave real sizes (the largest was 656.9 MB).
  - Also, `BrowseFileSharingApps` doesn't filter: it returned 442 apps, and only 21 set `UIFileSharingEnabled`.
- **Device identifiers in output.** ImageCaptureCore's error text contains the device name, and go-ios logs the UDID at INFO level, which leaked once into the session's terminal output (not into any file in the repo). Step 9 must report these errors by kind and keep go-ios's logging out of Groundskeeper's output.
- **The ImageCaptureCore `didRemove` callback arrives before the delete completion**, so the helper must wait for the completion.

### Decisions

- go-ios alone, no libimobiledevice fallback.
- Photo deletes on the phone are permanent; step 11 decides how strongly to guard them.
- The fixture recorder (the part that saves go-ios responses through `Scrub`) is built with the real go-ios code in step 9, so it records the requests step 9 actually makes.

### Verified

- `go test -race ./...` passes.
- Planting a file with a MAC address and a third-party bundle ID in a `testdata/` directory made the guard test fail, naming both.
- Making `Scrub` keep `WiFiAddress` made the scrubber tests fail, naming the MAC address.

### Open

- Why one app's data size differs from iPhone Storage by 18%.
- PhotoKit `delete` and Vision `analyze` are untested (steps 10 and 11).

## After step 7: private launchd log

- **Problem:** `~/Library/Logs/groundskeeper.launchd.log` was `-rw-r--r--`, readable by other users, while every other file was `0600`. launchd created it with default permissions. It holds the scan summary: free space, report path and largest item names.
- **Fix:**
  - `make install` creates the file with mode `0600` before loading the agent.
  - `doctor` has a new "Log files" check that warns if either log can be read by others, with the `chmod 600` fix. A failing test came first.
- **Verified:** a real scheduled run appended to the file (965 to 1,447 bytes) and it stayed `-rw-------`. `gk doctor` reports "readable only by you".

## Step 7: Schedule, notification, doctor, help topics and completion

### Built

- **`launchd/com.lot86.groundskeeper.plist`:** runs `~/bin/groundskeeper scan --notify` on Mondays and Thursdays at 09:00, logging to `~/Library/Logs/groundskeeper.launchd.log`. `make install` fills in `__HOME__`, lints the plist with `plutil`, and loads it with `launchctl bootstrap`; `make uninstall` unloads it with `bootout` and removes it.
- **`internal/notify`:** a notification in Rook's voice ("Rook's report is ready: 118.0 GB free, about 35.0 GB to tidy."), adding failed scanners and newly unreadable paths when there are any. The text goes to `osascript` as `argv`, never into the script.
- **`gk doctor` (new Setup group):** checks the installed binary, Full Disk Access, the schedule and its next run, the last scheduled run, FileVault, and whether the report folder syncs. Each problem comes with its fix.
- **Help topics** `safety`, `schedule` and `privacy`, grouped under "Help topics", with "See also" lines on `scan`, `clean browsers` and `doctor`.
- `--browser` completes only browsers that have folders on this Mac.
- `scan` warns when the report folder syncs off this Mac.
- Snapshots record skipped paths, so a scheduled run can report newly unreadable ones.

### Decisions

- The schedule passes `--notify`, rather than notifying based on whether the run looks interactive.
- Notifications on every scheduled run, Mon and Thu 09:00. This settles two of the plan's open decisions.
- iPhone-related `doctor` checks (tunnel, Photos access, `photos-helper`) wait for iPhone mode.

### Problems found and fixed (all from a real launchd run started with `launchctl kickstart`)

- **Scheduled runs were recorded as interactive.**
  - Root cause: `ui.IsTerminal` only checked for a character device, and launchd's stdin is `/dev/null`, which is a character device (`crw-rw-rw-`) but not a terminal (`isatty` false).
  - Fix: a failing test first, then `term.IsTerminal` from `golang.org/x/term`.
  - Correction to step 6: there I wrote that `go test` runs with a terminal as stdin. More likely its stdin was also `/dev/null`, misjudged by the same bug.
- **Scheduled report incomplete.**
  - With `ProcessType` `Background` (my addition, not in the plan), the run took about 7 minutes instead of 26 s.
  - `~/Library/Caches/com.apple.Music` and `com.apple.TV` hit the two-minute folder timeout.
  - Fix: remove `ProcessType` (launchd's default is light limits). Re-run: about 30 s, 0 timeouts.
- **The notification reported "26 new paths couldn't be read"** because older snapshots had no skipped list. Fix: snapshots always write the list, and a missing one counts as unknown (with a test).
- **`doctor` overstated Full Disk Access:** it said Downloads would be skipped, but Downloads was readable. Downloads, Documents and Desktop are under macOS's separate "Files and Folders" permission.
- **Fang listed the help topics under the generic Commands heading,** so they got their own group.

### Verified

- On a real Mac:
  - `make install` loaded the agent; `kickstart` ran it with exit code 0.
  - The snapshot shows `interactive: false`, Podman was found under launchd's `PATH`, and the notification was sent.
  - `gk doctor` reports the next run (Mon 26 Oct 09:00).
- `fdesetup status` works without `sudo`.
- A real `osascript` notification whose text tried `do shell script "touch …"` created no file.
- Tests:
  - the plist template lints, runs only `scan --notify`, and contains no hardcoded home
  - `doctor` (schedule states, next run, FileVault on and off, last scheduled run, synced folders including symlinks)
  - notifications (text and argument handling), `--notify` and dry runs
  - help topics and `--browser` completion

### Open

- Notifications come from Script Editor; if they don't appear, allow Script Editor in System Settings > Notifications.
- Full Disk Access for the schedule has to be granted to `~/bin/groundskeeper` by hand. `doctor` can only test the process running it.
- `make install` prints a harmless "Error 3 (ignored)" when there's no earlier job to unload.

## Between steps 6 and 7: on-demand install

- `make install` builds and copies the binary to `~/bin`, adds the `gk` symlink, and writes zsh completion for both names to `~/.zsh/completions/_groundskeeper` (the `#compdef` and `compdef` lines are rewritten to include `gk`). `make uninstall` removes exactly those three files. The LaunchAgent comes in step 7.
- Shell setup, outside the repo: `~/bin` was not on `PATH`, so it was added, and the completions folder joined `fpath` before oh-my-zsh runs `compinit`.
- **Problem found:** `gk` ran `gitk`. Root cause: oh-my-zsh's `git` plugin defines `alias gk='\gitk --all --branches &!'`, and aliases win over `PATH`. gitk isn't installed, so the alias did nothing useful. It is removed with `unalias gk` after oh-my-zsh loads. Anyone else using oh-my-zsh's `git` plugin will hit the same conflict.
- **Verified:** in a fresh interactive shell, `type gk` resolves to `~/bin/gk`, and `gk --version` and `gk stats` work from any folder.

## Step 6: History, deltas and metrics

### Built

- **`internal/history`:** each scan saves `~/.local/state/groundskeeper/YYYY-MM-DDTHHMM.json` (`0600`, folder `0700`). Each clean run saves `clean-YYYY-MM-DDTHHMM.json`. Snapshots older than six months are pruned, and only files whose names match those patterns are ever removed. `Compare` works out the change in free space, the change per item, and the five fastest-growing items (new items count from zero).
- **`internal/metrics`:**
  - per run: start, duration, command, whether run from a terminal
  - per scanner: time, items, bytes, skipped, timeouts, error kind
  - size engine: files and folders visited, hardlinks counted once
  - per clean run: moved, refused and failed counts, and bytes
- **Report:**
  - a "Free space since" card and a Change column (red when an item grew)
  - a "Growing fastest" section
  - a "Run details" section: a timing bar per scanner, slow (a minute or more) or failed scanners highlighted, size-engine counts, and space freed by clean commands since the last report
- **Terminal summary:** the change in free space since the last run, and each largest item's change.
- **New commands:** `history` (recent scans plus fastest-growing items), `stats` (last run, `--runs N` trend, `--json`) and `logs` (`--level`, `--run last`, `--cmd`, `-n`, `--follow`, `--json`).
- **Shared helpers:** `ui.Table` (Lip Gloss rounded table in a terminal, plain aligned columns otherwise), `ui.Delta` and `ui.Grew`.
- **Clean runs** now log their own start and end with totals.

### Decisions

- Failed scanners are recorded per scanner instead of as an exit status, because the run still finishes and saves a snapshot.
- "Interactive" means stdin is a terminal, which is false for launchd jobs.
- `--dry` reads history to show changes but never writes a snapshot.
- Two scans in the same minute share a snapshot name, and the later one replaces the earlier.

### Problems found and fixed

- **Clean runs with nothing to do left no log lines.** A `logs` test exposed it, against the plan's "start and end of each run". Clean now logs `run started` and `run finished`.
- **`logs` printed sizes in scientific notation** (`bytes=4.3041574912e+10`) on a real Mac, because JSON numbers decode as floats. A failing test came first, then the fix: decoding with `UseNumber`.
- **"Since" dates were ambiguous for scans on the same day,** so they now include the time.
- **Two wrong test expectations (code fine):** `html/template` escapes `+` as `&#43;`, and `go test` runs with a terminal as stdin, so `Interactive` is true there.

### Verified

- `stats --json` output matches the saved snapshot's metrics byte for byte (CLI test).
- Pruning removes expired snapshots and keeps unrelated files, look-alike names and folders (unit and CLI tests).
- On a real Mac:
  - Two scans a minute apart produced private snapshots, a change summary and a Growing fastest list.
  - `stats --runs 2` showed per-scanner times across both runs.
  - `logs --follow` printed a concurrent clean run's entries and exited cleanly on Ctrl-C.
- `logs` showed macOS blocking another app's `~/Library/Containers` folder with a permission error, which is recorded as skipped. This resolves the step 4 question: no prompt appeared.

### Open

- `logs` reads the current log file only, not the rotated `.1` to `.3` files.
- `--follow --run last` with no earlier entries matches nothing until a run's ID is known.

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
