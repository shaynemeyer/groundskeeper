# Build log

One entry per build step from [the plan](groundskeeper-plan.md): what was built, decisions made, problems found and how they were proven, and what is still open. Newest first.

## Step 11: photo grouping, review and clean photos

### Built

- **Increment 1:** `internal/iphone/photos.go` runs `photos-helper` (`Helper{Path, Source, Run}`) and decodes `list` into `[]Asset` and `analyze` into `[]Analysis` plus `[]Failure`. An `{"error": kind}` answer becomes `ErrNoDevice`, `ErrLocked`, `ErrDenied` or `ErrTimeout`; a helper that dies without one is `ErrHelper`. `Distance` is Vision's feature-print distance, the squared Euclidean distance of the float32 vectors (step 10).
- **Increment 2:** `photos-helper hash --source <source>` reads asset IDs on stdin and writes `{"source","hashes":[{id, sha256}],"failed":[{id, reason}]}`, with the new reason `read`. The device path reads each file in 4 MB chunks with `ICCameraFile.requestReadData(atOffset:length:)`; the library path streams the original `PHAssetResource` (the one `list` counts as `bytes`, now shared as `originalResource`) through `PHAssetResourceManager.requestData` with network access off. `Helper.Hash` is the Go side.
- **Increment 3:** `NearDuplicates` (`internal/iphone/group.go`) sorts photos by date, compares every pair taken within the window and every pair in the same burst, and joins pairs at most `NearDistance` (0.025) apart with a union-find, so links chain. Groups hold two or more IDs, oldest first. Photos without an analysis are left out, and photos without a date are only compared within their burst.
- **Increment 4:** `ExactCandidates` (`internal/iphone/exact.go`) returns the assets that share their size and dimensions with another, the only ones worth hashing. `ExactDuplicates` groups assets whose size, dimensions and SHA-256 all match, oldest first; assets without a hash or a size are left out.
- **Increment 5:** `Keep` (`internal/iphone/best.go`) returns a group's keepers: every protected asset (`Asset.Protected`: favorite, edited or in an album) plus the best shot, the highest aesthetics score. With protected assets in the group the best shot is chosen among them. A missing score ranks lowest, and ties go to the older photo.
- **Increment 6, old screenshots and the largest videos.** First, device `list` folds each edited render (`IMG_E0001.JPG`) into its original (`IMG_0001.HEIC`) when exactly one original is in the same folder with the same creation date: the render is no longer an asset, its size goes to the original's `sidecarBytes`, and the original is `edited: true` (`deviceAssets` in `Asset.swift`). Then `list` gains `video` and `screenshot` on both sources: on the phone from the UTI (`public.movie`) and EXIF `UserComment` (`Screenshot`, from `requestMetadataDictionary`, metadata only), in the library from `mediaType` and the `photoScreenshot` subtype. Last, Go's `Asset` gains `Video` and `Screenshot`, and `internal/iphone/categories.go` adds `OldScreenshots` (screenshots older than a number of days, oldest first) and `LargestVideos` (the n largest videos counting sidecars, ties to the older one). Both leave out protected assets, and `OldScreenshots` leaves out undated ones.

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

### Problems found

- **Library `list` counted every original twice.** After increment 2, the two unedited library JPEGs had `sidecarBytes` equal to `bytes`. A read-only probe outside the repo listed each asset's PhotoKit resources: those two had only their `photo` resource, and the edited screenshot's `photo`, `adjustmentData` and `fullSizePhoto` came to 32,522 + 2,048 + 12,467 = 47,037 bytes, exactly the `sidecarBytes` `list` gave, original included. Cause: increment 2 moved the original's lookup into `originalResource(of:)`, which called `assetResources(for:)` again, and that returns new objects, so `Asset`'s identity check (`!==`) never matched the original. Fixed by having `originalResource(in:)` pick from the array the caller already holds. Afterwards `sidecarBytes` was 0, 0, 14,515 and 0, matching the probe, and the three hashes were unchanged.

### Open

- Decoding hasn't been run against the real helper's output yet; that comes when `gk iphone` runs it.
- `hash --source mac-library` ran from a terminal app with Photos access and hashed the 3 oldest library photos (two JPEGs and an edited PNG screenshot) with no failures; from the agent's terminal it answers `denied`. Each hash equalled `shasum -a 256` of the photo's file in the library's `originals` folder (named by the ID's UUID), and each file's size equalled `list`'s `bytes`. The edited screenshot hashed its unedited original, as intended.
- An edited library photo's render is reported, as PhotoKit's `fullSizePhoto` resource (12,467 bytes on the edited screenshot), so it's counted in `sidecarBytes`. This answers step 10's open question.
- Vision's ranking doesn't always match the user's eye: of the first 14 pairs, the user found the one at 0.075 more alike than the one at 0.048.
- Every pair the user called a duplicate was taken within 2 seconds, and every pair 12 s or more apart wasn't. A time rule might separate the mixed 0.026 to 0.048 range, but that's 39 pairs on one phone, so the 10-minute window stays.
- The best-shot check couldn't test a sharp shot against a blurry one: at the 0.025 cutoff the grouped photos are nearly identical. If reviews show bad picks, sharpness (not a Vision request; would need its own measure) and Vision's face capture quality are the candidates.
- Both copies of the library's exact pair report `inAlbum: true`, so `Keep` keeps both and the group has nothing to delete. The user only dragged them into the library, but a read-only probe (`fetchAssetCollectionsContaining`, album subtypes only) found every screenshot in the library, both copies included, in a `regular` album, the kind a person makes, and none in an imported or synced one. So the flag and the rule are right; how the copies got into that album (perhaps it was open in the sidebar during the drop) isn't known.
- An exact pair taken at the same moment would also be a near-duplicate group. The review has to merge or de-duplicate the two kinds of group.
- Exact duplicates on the phone itself weren't tested, since it has none. Whether Photos' Duplicate command on the phone makes a byte-identical copy isn't known.
- Video poster frames are grouped like photos; none of the 12 groups mixed a video with a photo, but nothing prevents it.
- Over AFC the camera roll's folders are `100APPLE` to `122APPLE`, while ImageCaptureCore names them `YYYYMM_a`, so AFC paths and helper IDs share only the file name.
- Whether deleting an edited original on the device path also removes its render is untested. Edited photos are protected, so nothing suggests deleting one; test with a throwaway edited photo before that changes.
- How many largest videos to show isn't decided; `LargestVideos` takes n, and the review or report will pick it.
- Whether any of the 12 near-duplicate groups found before increment 6 paired an original with its render wasn't checked; renders can't be grouped now.

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
