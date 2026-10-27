package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

const marker = "GK-SECRET-MARKER-7f3a"

func fakeHome(t *testing.T) string {
	t.Helper()
	// clean mac refuses paths through a symlink, and t.TempDir is under
	// /var, a symlink to /private/var.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NO_COLOR", "")
	appDirs = func(home string) []string { return []string{filepath.Join(home, "Applications")} }
	toolRunner = func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
	t.Cleanup(func() {
		appDirs = func(string) []string { return nil }
		toolRunner = nil
	})
	write := func(rel string, data []byte) {
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".npm/cache", make([]byte, 50_000))
	write(".config/groundskeeper/config.json", []byte(`{"minSizeMB": 0}`))
	profile := "Library/Application Support/Google/Chrome/Default/"
	for _, f := range []string{"Cookies", "Login Data", "History"} {
		write(profile+f, []byte(marker))
	}
	return home
}

func run(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRoot()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestEveryCommandHasHelpContent(t *testing.T) {
	var check func(*cobra.Command)
	check = func(c *cobra.Command) {
		if c.Short == "" || c.Long == "" || (c.Runnable() && c.Example == "") {
			t.Errorf("%q needs Short, Long, and Example if runnable", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			if sub.Name() != "help" && sub.Name() != "completion" {
				check(sub)
			}
		}
	}
	check(NewRoot())
}

func TestScanWritesPrivateReportAndLog(t *testing.T) {
	home := fakeHome(t)
	out := run(t, "scan")

	report := filepath.Join(home, "Reports/groundskeeper/latest.html")
	logFile := filepath.Join(home, "Library/Logs/groundskeeper.log")
	for _, p := range []string{report, logFile} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", p, info.Mode().Perm())
		}
	}
	if !strings.Contains(out, "npm") || !strings.Contains(out, "Report") {
		t.Errorf("summary = %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("non-terminal output has escape codes: %q", out)
	}
}

func TestNoArgsRunsScan(t *testing.T) {
	home := fakeHome(t)
	run(t)
	if _, err := os.Stat(filepath.Join(home, "Reports/groundskeeper/latest.html")); err != nil {
		t.Fatal(err)
	}
}

func TestDryWritesNoFiles(t *testing.T) {
	home := fakeHome(t)
	out := run(t, "scan", "--dry")

	if !strings.Contains(out, "Dry run: no files were written.") {
		t.Errorf("summary = %q", out)
	}
	for _, p := range []string{"Reports", "Library/Logs"} {
		if _, err := os.Stat(filepath.Join(home, p)); !os.IsNotExist(err) {
			t.Errorf("%s exists after --dry", p)
		}
	}
}

func TestDryVerbosePrintsToStderrOnly(t *testing.T) {
	home := fakeHome(t)
	out := run(t, "scan", "--dry", "--verbose")

	if !strings.Contains(out, "scanner done") {
		t.Errorf("verbose output missing scanner lines: %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, "Library/Logs")); !os.IsNotExist(err) {
		t.Error("log written during --dry")
	}
}

func TestScanFindsBuildArtifacts(t *testing.T) {
	home := fakeHome(t)
	nm := filepath.Join(home, "projects/app/node_modules/x")
	if err := os.MkdirAll(filepath.Dir(nm), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nm, make([]byte, 50_000), 0o600); err != nil {
		t.Fatal(err)
	}
	out := run(t, "scan", "--dry")

	if !strings.Contains(out, "projects/app/node_modules") {
		t.Errorf("summary missing artifact: %q", out)
	}
}

func TestScanSectionsInReportOrder(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	data, err := os.ReadFile(filepath.Join(home, "Reports/groundskeeper/latest.html"))
	if err != nil {
		t.Fatal(err)
	}
	html, last := string(data), -1
	for _, section := range []string{"Caches", "AI models", "Build artifacts", "Toolchains"} {
		i := strings.Index(html, "<span>"+section+"</span>")
		if i < last {
			t.Errorf("section %q missing or out of order", section)
		}
		last = i
	}
}

func chromeCache(t *testing.T, home string) string {
	t.Helper()
	cache := filepath.Join(home, "Library/Caches/Google/Chrome/Default/Cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "data_1"), make([]byte, 50_000), 0o600); err != nil {
		t.Fatal(err)
	}
	return cache
}

// fakeTools answers ps with procs and records trash calls.
func fakeTools(t *testing.T, procs string) *[]string {
	t.Helper()
	var trashed []string
	toolRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "ps":
			return []byte(procs), nil
		case "/usr/bin/trash":
			trashed = append(trashed, args[len(args)-1])
			return nil, nil
		}
		return nil, exec.ErrNotFound
	}
	return &trashed
}

func TestCleanBrowsersDryRunByDefault(t *testing.T) {
	home := fakeHome(t)
	chromeCache(t, home)
	trashed := fakeTools(t, "")
	out := run(t, "clean", "browsers", "--browser", "chrome")

	if !strings.Contains(out, "Dry run: nothing was changed. Add --apply to move") || !strings.Contains(out, "would move") {
		t.Errorf("output = %q", out)
	}
	if len(*trashed) != 0 {
		t.Errorf("dry run trashed %v", *trashed)
	}
	data, _ := os.ReadFile(filepath.Join(home, "Library/Logs/groundskeeper.log"))
	if !strings.Contains(string(data), `"dry_run":true`) {
		t.Error("dry run not logged")
	}
}

func TestCleanBrowsersApplyMovesToTrash(t *testing.T) {
	home := fakeHome(t)
	cache := chromeCache(t, home)
	trashed := fakeTools(t, "")
	out := run(t, "clean", "browsers", "--apply")

	if len(*trashed) != 1 || (*trashed)[0] != cache {
		t.Fatalf("trashed %v, want %s", *trashed, cache)
	}
	if !strings.Contains(out, "Moved") || !strings.Contains(out, "to the Trash.") {
		t.Errorf("output = %q", out)
	}
}

func TestCleanBrowsersRunningBrowserMessage(t *testing.T) {
	home := fakeHome(t)
	chromeCache(t, home)
	trashed := fakeTools(t, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome\n")
	out := run(t, "clean", "browsers", "--apply")

	if !strings.Contains(out, "Chrome is running, so it was skipped. Quit it and run the command again.") {
		t.Errorf("output = %q", out)
	}
	if len(*trashed) != 0 {
		t.Errorf("trashed %v while Chrome was running", *trashed)
	}
}

func stateDir(home string) string { return filepath.Join(home, ".local/state/groundskeeper") }

func readSnapshots(t *testing.T, home string) []history.Snapshot {
	t.Helper()
	scans, err := history.Scans(stateDir(home))
	if err != nil {
		t.Fatal(err)
	}
	return scans
}

func TestSecondScanShowsChanges(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	if err := os.WriteFile(filepath.Join(home, ".npm/more"), make([]byte, 200_000), 0o600); err != nil {
		t.Fatal(err)
	}
	out := run(t, "scan")

	if !strings.Contains(out, "free space") || !strings.Contains(out, "npm  (+") {
		t.Errorf("summary = %q", out)
	}
	data, _ := os.ReadFile(filepath.Join(home, "Reports/groundskeeper/latest.html"))
	if !strings.Contains(string(data), "Growing fastest") {
		t.Error("report has no Growing fastest section")
	}
}

func TestDryScanSavesNoSnapshot(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan", "--dry")
	if _, err := os.Stat(stateDir(home)); !os.IsNotExist(err) {
		t.Fatal("--dry wrote a snapshot")
	}
}

func TestSnapshotRecordsScannerMetrics(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")

	scans := readSnapshots(t, home)
	if len(scans) != 1 {
		t.Fatalf("got %d snapshots", len(scans))
	}
	r := scans[0].Run
	if r.Command != "scan" || len(r.Scanners) != 8 || r.Size.Files == 0 {
		t.Fatalf("run metrics = %+v", r)
	}
	caches := r.Scanners[0]
	if caches.Name != "Caches" || caches.Items != 1 || caches.Bytes == 0 {
		t.Errorf("caches metrics = %+v", caches)
	}
	info, err := os.Stat(filepath.Join(stateDir(home), scans[0].Time.Format("2006-01-02T1504")+".json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("snapshot file: %v %v", info, err)
	}
}

func TestSnapshotRecordsTrashAndSafety(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")

	items := readSnapshots(t, home)[0].Items
	i := slices.IndexFunc(items, func(it history.Item) bool { return it.Label == "npm" })
	if i < 0 {
		t.Fatalf("no npm row in %+v", items)
	}
	if items[i].Trash != filepath.Join(home, ".npm") || items[i].Safety != "safe" {
		t.Errorf("npm row = %+v", items[i])
	}
}

func TestSnapshotRecordsSkippedPaths(t *testing.T) {
	home := fakeHome(t)
	locked := filepath.Join(home, ".npm/locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })
	run(t, "scan")

	s := readSnapshots(t, home)[0]
	if caches := s.Run.Scanners[0]; caches.Name != "Caches" || caches.Skipped != 1 {
		t.Errorf("caches metrics = %+v, want 1 skipped", caches)
	}
	if !slices.Contains(s.Skipped, locked) {
		t.Errorf("snapshot skipped = %q, want %s", s.Skipped, locked)
	}
}

func TestAppliedCleanShowsAsFreedInNextReport(t *testing.T) {
	home := fakeHome(t)
	chromeCache(t, home)
	fakeTools(t, "")
	run(t, "clean", "browsers", "--apply")
	cleans, _ := history.Cleans(stateDir(home), time.Time{})
	if len(cleans) != 1 || cleans[0].DryRun || cleans[0].Moved != 1 {
		t.Fatalf("clean snapshots = %+v", cleans)
	}

	run(t, "scan")
	data, _ := os.ReadFile(filepath.Join(home, "Reports/groundskeeper/latest.html"))
	if !strings.Contains(string(data), "Clean commands moved") {
		t.Error("report doesn't mention space freed by clean")
	}
}

func TestScanPrunesOnlyExpiredSnapshots(t *testing.T) {
	home := fakeHome(t)
	dir := stateDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2025-01-01T0900.json", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600)
	}
	run(t, "scan")

	if _, err := os.Stat(filepath.Join(dir, "2025-01-01T0900.json")); !os.IsNotExist(err) {
		t.Error("expired snapshot kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Error("unrelated file removed")
	}
}

func TestHistoryListsScansAndGrowth(t *testing.T) {
	home := fakeHome(t)
	if out := run(t, "history"); !strings.Contains(out, "No scans yet") {
		t.Errorf("empty history = %q", out)
	}
	writeSnapshot(t, home, -48*time.Hour, 50_000)
	run(t, "scan")

	out := run(t, "history")
	for _, want := range []string{"Scan", "Free", "Change", "Growing fastest since", "npm (Caches)"} {
		if !strings.Contains(out, want) {
			t.Errorf("history missing %q:\n%s", want, out)
		}
	}
}

// writeSnapshot saves an earlier scan in which npm was smaller.
func writeSnapshot(t *testing.T, home string, ago time.Duration, npmBytes int64) {
	t.Helper()
	s := history.Snapshot{Time: time.Now().Add(ago), Free: 1, Total: 2, Items: []history.Item{
		{Section: "Caches", Label: "npm", Path: filepath.Join(home, ".npm"), Bytes: npmBytes},
	}}
	if err := history.SaveScan(stateDir(home), s); err != nil {
		t.Fatal(err)
	}
}

func TestStatsJSONMatchesSnapshot(t *testing.T) {
	home := fakeHome(t)
	run(t, "scan")
	out := run(t, "stats", "--json")

	var got []metrics.Run
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	want, _ := json.Marshal([]metrics.Run{readSnapshots(t, home)[0].Run})
	gotJSON, _ := json.Marshal(got)
	if string(gotJSON) != string(want) {
		t.Fatalf("stats --json\n%s\nsnapshot\n%s", gotJSON, want)
	}
}

func TestStatsShowsLastRunAndTrend(t *testing.T) {
	home := fakeHome(t)
	writeSnapshot(t, home, -48*time.Hour, 1)
	run(t, "scan")

	out := run(t, "stats")
	for _, want := range []string{"Last scan", "Caches", "Unused apps", "Visited", "Clean runs since: 0 applied"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q:\n%s", want, out)
		}
	}
	trend := run(t, "stats", "--runs", "2")
	if lines := strings.Split(strings.TrimSpace(trend), "\n"); len(lines) != 9 || !strings.Contains(lines[1], "-") {
		t.Errorf("trend =\n%s", trend)
	}
}

func TestLogsFiltersAndJSON(t *testing.T) {
	fakeHome(t)
	fakeTools(t, "")
	run(t, "scan")
	run(t, "clean", "browsers")

	all := run(t, "logs", "-n", "500")
	if !strings.Contains(all, "scanner done") || !strings.Contains(all, "clean browsers") {
		t.Errorf("logs = %q", all)
	}
	scanOnly := run(t, "logs", "--cmd", "scan", "-n", "500")
	if strings.Contains(scanOnly, "clean browsers  ") {
		t.Errorf("--cmd scan shows clean entries:\n%s", scanOnly)
	}
	for _, line := range strings.Split(strings.TrimSpace(run(t, "logs", "--level", "warn", "-n", "500")), "\n") {
		if line != "" && !strings.Contains(line, "WARN") && !strings.Contains(line, "ERROR") {
			t.Errorf("--level warn shows %q", line)
		}
	}
	raw := run(t, "logs", "--json", "--run", "last")
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if !json.Valid([]byte(line)) || !strings.Contains(line, `"cmd":"clean browsers"`) {
			t.Errorf("--json --run last line %q", line)
		}
	}
}

// fakeOsascript records notification text.
func fakeOsascript(t *testing.T) *[]string {
	t.Helper()
	var sent []string
	toolRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "osascript" {
			sent = append(sent, args[len(args)-2])
			return nil, nil
		}
		return nil, exec.ErrNotFound
	}
	return &sent
}

func TestNotifyPostsOneNotification(t *testing.T) {
	fakeHome(t)
	sent := fakeOsascript(t)
	run(t, "scan", "--notify")

	if len(*sent) != 1 || !strings.HasPrefix((*sent)[0], "Rook's report is ready:") {
		t.Fatalf("notifications = %q", *sent)
	}
}

func TestNotifyMentionsTimeoutsEveryRun(t *testing.T) {
	fakeHome(t)
	sent := fakeOsascript(t)
	folderTimeout = time.Nanosecond
	t.Cleanup(func() { folderTimeout = 2 * time.Minute })
	run(t, "scan", "--notify")
	run(t, "scan", "--notify")

	if len(*sent) != 2 {
		t.Fatalf("notifications = %q", *sent)
	}
	for i, msg := range *sent {
		if !strings.Contains(msg, "had problems") {
			t.Errorf("run %d notification %q doesn't mention the timeout", i+1, msg)
		}
	}
}

func TestNoNotificationWithoutFlagOrOnDryRun(t *testing.T) {
	fakeHome(t)
	sent := fakeOsascript(t)
	run(t, "scan")
	run(t, "scan", "--dry", "--notify")

	if len(*sent) != 0 {
		t.Fatalf("notifications = %q", *sent)
	}
}

func TestHelpTopics(t *testing.T) {
	fakeHome(t)
	out := run(t, "--help")
	for _, topic := range []string{"safety", "schedule", "privacy"} {
		if !strings.Contains(out, topic) {
			t.Errorf("help doesn't list %s", topic)
		}
	}
	if out := run(t, "help", "privacy"); !strings.Contains(out, "Full Disk Access") || !strings.Contains(out, "Time Machine") {
		t.Errorf("privacy topic = %q", out)
	}
}

func TestDoctorReportsFixes(t *testing.T) {
	fakeHome(t)
	var out bytes.Buffer
	root := NewRoot()
	root.SetArgs([]string{"doctor"})
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.ExecuteContext(context.Background())

	if err == nil || !strings.Contains(err.Error(), "check(s) failed") {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"fail  Installed binary", "Fix: Run make install", "LaunchAgent not installed", "Report folder"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestScanWarnsAboutSyncedReportFolder(t *testing.T) {
	home := fakeHome(t)
	cfg := filepath.Join(home, ".config/groundskeeper/config.json")
	if err := os.WriteFile(cfg, []byte(`{"minSizeMB": 0, "outputDir": "~/Documents/gk-reports"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := run(t, "scan", "--dry"); !strings.Contains(out, "Warning: the report folder is in Documents") {
		t.Errorf("output = %q", out)
	}
}

func TestBrowserFlagCompletesInstalledBrowsers(t *testing.T) {
	home := fakeHome(t)
	chromeCache(t, home)
	out := run(t, "__complete", "clean", "browsers", "--browser", "")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !slices.Contains(lines, "chrome") || slices.Contains(lines, "firefox") {
		t.Fatalf("completions = %q", out)
	}
}

func TestNoContentsOrSecretsInOutput(t *testing.T) {
	home := fakeHome(t)
	t.Setenv("GITHUB_TOKEN", marker)
	out := run(t, "scan", "--verbose")

	if strings.Contains(out, marker) {
		t.Error("marker in stdout or stderr")
	}
	for _, p := range []string{"Reports/groundskeeper/latest.html", "Library/Logs/groundskeeper.log"} {
		data, err := os.ReadFile(filepath.Join(home, p))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(marker)) {
			t.Errorf("marker in %s", p)
		}
	}
	if len(readSnapshots(t, home)) == 0 {
		t.Fatal("scan wrote no snapshot")
	}
	state, err := os.ReadDir(stateDir(home))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range state {
		data, err := os.ReadFile(filepath.Join(stateDir(home), e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(marker)) {
			t.Errorf("marker in state file %s", e.Name())
		}
	}
}

func TestBadConfigFails(t *testing.T) {
	home := fakeHome(t)
	cfg := filepath.Join(home, "bad.json")
	if err := os.WriteFile(cfg, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRoot()
	root.SetArgs([]string{"scan", "--config", cfg})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil {
		t.Fatal("want error for invalid config")
	}
}
