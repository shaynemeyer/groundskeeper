# Homebrew tap plan

Install Groundskeeper with `brew install shaynemeyer/tap/groundskeeper` instead of cloning and running `make install`. Homebrew builds it from source on the user's Mac, so nothing pre-built is downloaded: no Gatekeeper prompt, no Developer ID or notarization, and Intel Macs that run macOS 26 are covered too. Signed, pre-built releases stay a later, separate decision.

This is build step 18 in [the plan](groundskeeper-plan.md#build-order).

## What the user gets

```sh
brew install shaynemeyer/tap/groundskeeper
gk schedule on      # Monday and Thursday scans, as make install sets up today
gk doctor
```

- `groundskeeper`, `gk` and `photos-helper` in Homebrew's `bin` (`/opt/homebrew/bin` on Apple Silicon).
- zsh completion for both names.
- `brew upgrade` to update and `brew uninstall` to remove (after `gk schedule off`).

`make install` stays for development and works as before.

## Findings that shape the plan

Checked against Homebrew 7.0.11 on this Mac.

- **`brew services` can't keep the schedule.** The formula `service` block's `cron` turns each field into one integer (`Integer(part)` in `Library/Homebrew/service.rb`, `parse_cron`), so `0 9 * * 1,4` fails: `Integer("1,4")` raises `ArgumentError`. It also writes a single `StartCalendarInterval` entry, and the schedule needs two (Monday and Thursday). So Groundskeeper manages its own LaunchAgent with a new `gk schedule` command, used by both install routes. Docs: [Formula Cookbook, service files](https://docs.brew.sh/Formula-Cookbook#service-files).
- **Homebrew builds Go with `GOTOOLCHAIN=local`** (`Library/Homebrew/extend/ENV/super.rb:129`). The build fails if Homebrew's `go` is older than the `go` line in `go.mod`. Both are 1.27.3 today. Upgrading `go.mod` to a patch release Homebrew doesn't have yet breaks `brew install` until Homebrew catches up.
- **`depends_on macos: :tahoe`** is a known symbol (`MacOSVersion::SYMBOLS`), matching `photos-helper`'s `.macOS(.v26)`.
- **Completions:** `generate_completions_from_executable` takes `shell_parameter_format: :cobra` and `shells:` (`Library/Homebrew/formula.rb`, `utils/shell_completion.rb`).
- **Tap layout:** the repository is `shaynemeyer/homebrew-tap` with formulae in `Formula/`. Users install with `brew install shaynemeyer/tap/groundskeeper`. Docs: [How to create and maintain a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap).
- **The code assumes `~/bin`.** `doctor.HelperPath` (used by `iphone`, `review photos` and `clean photos`), `doctor`'s install and schedule checks, the LaunchAgent template and `gk help schedule` all name `~/bin/groundskeeper`.
- **Licence:** MIT (`LICENSE`), Rook artwork included, so the formula uses `license "MIT"`.

## Spike (about half a day)

Answer these before writing code, and record the answers in the build log.

**Results** (details in the [build log](build-log.md#step-18-spike-homebrew-tap)): (2) `os.Executable()` keeps the link path, so the plist and the helper lookup use it as is. (3) The Command Line Tools are enough, and `--disable-sandbox` is required. (4) `brew audit` raised nothing beyond the spike's own URL. (1) Partly answered: a Full Disk Access grant didn't survive a simulated `brew upgrade`, so Homebrew users will probably need to grant it again after each upgrade. Confirm that with a real `brew upgrade` in step 9, and if it holds, have the formula's caveats and `gk doctor` say so.

1. **Full Disk Access across upgrades.** macOS keys the grant to the binary. Homebrew installs into a versioned Cellar folder (`Cellar/groundskeeper/<version>/bin`) and links it from `/opt/homebrew/bin`. Grant Full Disk Access, rebuild with a new version, and check whether the scheduled scan still reads `~/Downloads`. Try it with the grant on the `/opt/homebrew/bin` link and on the Cellar binary. The same question applies to `make install` today, which rebuilds the ad-hoc signed binary each time.
2. **`os.Executable()` through a symlink.** What does it return when run as `/opt/homebrew/bin/gk`: the link or the Cellar path? `gk schedule` must write a path that survives `brew upgrade`, which removes the old Cellar folder.
3. **`swift build` inside Homebrew's build.** Does it work with only the Command Line Tools (no full Xcode)? Does SwiftPM need `--disable-sandbox`, since Homebrew already sandboxes the build?
4. **`brew audit --strict --new`** on a draft formula: what does it flag?

## Steps

Each step is small, tested, and leaves `make install` working.

### In this repository

1. **Find `photos-helper` next to the running binary.** `doctor.HelperPath` uses the folder of `os.Executable()` (resolved as the spike decides) instead of `~/bin`. Tests: a helper beside a fake executable is found; the `iphone`, `review photos` and `clean photos` hooks still get the path.
2. **`gk schedule on | off | status`** (Setup group). `on` writes `~/Library/LaunchAgents/com.lot86.groundskeeper.plist` from an embedded template with the binary's stable path (spike question 2). It creates the launchd log with `0600` first, then runs `launchctl bootout` and `bootstrap` through `exec.CommandContext` with separate arguments. `off` boots it out and removes the plist. `status` says whether it's loaded and which binary it runs. `make install` calls `groundskeeper schedule on` instead of its own `sed` and `launchctl` lines, so there's one code path. Tests use a fake home and a `launchctl` runner hook: the plist is valid (`plutil -lint`), has both calendar entries and the expected binary, is `0644`, and no `sh -c` is used. Help, the safety note, examples, `gk help schedule` and the user guide are updated in the same change, and `guide_test.go` checks them.
3. **`doctor` follows the running binary.** The install check looks at the running executable, not `~/bin/groundskeeper`. The schedule check reads the plist's `ProgramArguments[0]` and warns if that file is missing (for example after `brew uninstall` without `gk schedule off`) or differs from the running binary (both `make install` and Homebrew installed). The Full Disk Access advice names the path the spike shows is the one to grant.
4. **Completion for both names in Go.** `groundskeeper completion zsh` emits `#compdef groundskeeper gk` itself, so the `sed` in the Makefile goes and the formula needs no `inreplace`. Test: the first two lines name both commands.
5. **Licence** (done): MIT, in `LICENSE`, covering the Rook artwork too, and named in the README.
6. **Tag `v0.10.0`** and check the GitHub tarball builds: `go build` with `-X main.version=0.10.0`, because the tarball has no `.git` for `git describe`.

### In `shaynemeyer/homebrew-tap`

7. **Create the tap** with `brew tap-new shaynemeyer/tap`. Remove its bottle-building workflows: building from source is the point, and bottles would bring back the signing question.
8. **Write `Formula/groundskeeper.rb`:**

   ```ruby
   class Groundskeeper < Formula
     desc "Find disk space you can reclaim on your Mac and a connected iPhone"
     homepage "https://shaynemeyer.github.io/groundskeeper/"
     url "https://github.com/shaynemeyer/groundskeeper/archive/refs/tags/v0.10.0.tar.gz"
     sha256 "<shasum -a 256 of the tarball>"
     license "MIT"

     depends_on "go" => :build
     depends_on macos: :tahoe

     def install
       system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}"), "./cmd/groundskeeper"
       system "swift", "build", "--disable-sandbox", "-c", "release", "--package-path", "helpers/photos-helper"
       bin.install "helpers/photos-helper/.build/release/photos-helper"
       bin.install_symlink "groundskeeper" => "gk"
       generate_completions_from_executable(bin/"groundskeeper", "completion",
                                            shell_parameter_format: :cobra, shells: [:zsh])
     end

     def caveats
       <<~EOS
         Give #{opt_bin}/groundskeeper Full Disk Access in System Settings,
         then turn on the twice-weekly scan and check the setup:
           gk schedule on
           gk doctor
         Before uninstalling, run: gk schedule off
       EOS
     end

     test do
       assert_match version.to_s, shell_output("#{bin}/groundskeeper --version")
       assert_path_exists bin/"photos-helper"
     end
   end
   ```

   `--disable-sandbox` and the Full Disk Access path depend on the spike's answers.
9. **Try it locally before pushing.** Run `brew install --build-from-source shaynemeyer/tap/groundskeeper` from the local tap, then `brew test groundskeeper` and `brew audit --strict --new groundskeeper`. Check `gk --version`, `gk schedule on`, `gk doctor`, a scheduled run with `launchctl kickstart`, and `gk iphone` with the phone connected. Then check `brew uninstall` after `gk schedule off` leaves nothing behind except the state, report and log folders.
10. **Publish the tap** and install it on this Mac from GitHub, after `make uninstall`, to show the public path works.

### Docs

11. **README, user guide, site and `gk help`.** Homebrew becomes the first install route and building from source the second. The guide's install section covers `gk schedule on`, Full Disk Access for the Homebrew path, `brew upgrade` and uninstalling. The site's Install section and its "macOS 26, Xcode Command Line Tools, Go" chips change too, since Homebrew installs Go. Add a build log entry.

## Releasing a new version

1. Tag `vX.Y.Z` in this repository and push the tag.
2. In the tap, update `url` and `sha256` (`curl -sL <tarball> | shasum -a 256`), then run `brew audit` and `brew test`, commit and push.
3. Before tagging, check that Homebrew's `go` is at least the `go.mod` version (`brew info go`).

Automating step 2 with a GitHub Action is possible later. It's part of the CI decision in [next steps](next-steps.md#5-ci).

## Security notes

- Nothing changes at run time: no network access, the same read-only scans and the same permissions.
- At build time Homebrew downloads the tagged tarball (checked against `sha256`) and the Go modules in `go.sum`. `govulncheck` should pass on every tagged commit (`make test`).
- `gk schedule` runs `launchctl` with separate arguments, never through a shell, and writes only its own plist and log.

## Open decisions

- ~~Licence~~: decided, MIT, including the Rook artwork.
- **Tap name:** `homebrew-tap` (room for future tools) or `homebrew-groundskeeper`.
- **Schedule on install:** the formula can't load a LaunchAgent itself (Homebrew has no post-install step for user agents), so the user runs `gk schedule on`. Should `gk doctor` suggest it when the schedule is off?
- **Pre-built bottles or signed releases:** later, only if people use it. Both bring back Developer ID signing and notarization ($99 a year).
