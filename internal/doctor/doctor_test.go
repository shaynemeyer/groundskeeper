package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

// fake answers launchctl, plutil, fdesetup and photos-helper.
type fake struct {
	loaded    bool
	fileVault string
	plistJSON string
	access    string
}

func (f fake) run(_ context.Context, name string, _ ...string) ([]byte, error) {
	switch name {
	case "launchctl":
		if f.loaded {
			return nil, nil
		}
		return nil, errors.New("exit status 113")
	case "plutil":
		return []byte(f.plistJSON), nil
	case "fdesetup":
		return []byte(f.fileVault), nil
	}
	if filepath.Base(name) == "photos-helper" && f.access != "" {
		return []byte(`{"access":"` + f.access + `"}`), nil
	}
	return nil, errors.New("unexpected " + name)
}

const schedulePlist = `{"StartCalendarInterval":[{"Weekday":1,"Hour":9,"Minute":0},{"Weekday":4,"Hour":9,"Minute":0}]}`

// sunday is Sunday 25 October 2026, 18:00.
var sunday = time.Date(2026, 10, 25, 18, 0, 0, 0, time.Local)

func env(t *testing.T, f fake) Env {
	t.Helper()
	home := t.TempDir()
	return Env{Home: home, OutputDir: filepath.Join(home, "Reports/groundskeeper"), Run: f.run, Now: sunday}
}

func installPlist(t *testing.T, home string) {
	t.Helper()
	path := PlistPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<plist/>"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleStates(t *testing.T) {
	e := env(t, fake{})
	if c := schedule(context.Background(), e); c.Status != Fail || c.Detail != "LaunchAgent not installed" {
		t.Errorf("not installed = %+v", c)
	}
	installPlist(t, e.Home)
	if c := schedule(context.Background(), e); c.Status != Fail || !strings.Contains(c.Fix, "launchctl bootstrap") {
		t.Errorf("not loaded = %+v", c)
	}
	e.Run = fake{loaded: true, plistJSON: schedulePlist}.run
	if c := schedule(context.Background(), e); c.Status != OK || c.Detail != "loaded, next run Mon 26 Oct 09:00" {
		t.Errorf("loaded = %+v", c)
	}
}

func TestNextRunAfterMondayIsThursday(t *testing.T) {
	e := env(t, fake{plistJSON: schedulePlist})
	e.Now = time.Date(2026, 10, 26, 9, 30, 0, 0, time.Local)
	next, ok := nextRun(context.Background(), e, "x")
	if !ok || next.Format("Mon 2 Jan 15:04") != "Thu 29 Oct 09:00" {
		t.Fatalf("next = %v %v", next, ok)
	}
}

func TestFileVault(t *testing.T) {
	on := fileVault(context.Background(), env(t, fake{fileVault: "FileVault is On.\n"}))
	off := fileVault(context.Background(), env(t, fake{fileVault: "FileVault is Off.\n"}))
	if on.Status != OK || off.Status != Warn || !strings.Contains(off.Fix, "Turn On") {
		t.Fatalf("on = %+v, off = %+v", on, off)
	}
}

func TestLastScheduledRun(t *testing.T) {
	e := env(t, fake{})
	scheduled := func(ago time.Duration, failed bool) history.Snapshot {
		s := history.Snapshot{Time: sunday.Add(-ago), Run: metrics.Run{Interactive: false}}
		if failed {
			s.Run.Scanners = []metrics.Scanner{{Name: "Containers", Error: "timeout"}}
		}
		return s
	}
	cases := []struct {
		scans  []history.Snapshot
		status string
		detail string
	}{
		{nil, Warn, "none yet"},
		{[]history.Snapshot{scheduled(3*24*time.Hour, false)}, OK, "Thu 22 Oct 18:00"},
		{[]history.Snapshot{scheduled(10*24*time.Hour, false)}, Warn, "more than a week ago"},
		{[]history.Snapshot{scheduled(24*time.Hour, true)}, Warn, "1 scanner(s) failed"},
		{[]history.Snapshot{scheduled(24*time.Hour, false), {Time: sunday, Run: metrics.Run{Interactive: true}}}, OK, "Sat 24 Oct"},
	}
	for i, c := range cases {
		e.Scans = c.scans
		got := lastScheduledRun(e)
		if got.Status != c.status || !strings.Contains(got.Detail, c.detail) {
			t.Errorf("case %d: got %+v, want %s containing %q", i, got, c.status, c.detail)
		}
	}
}

func TestSyncedFolder(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"Documents", "Desktop", "Library/Mobile Documents", "Library/CloudStorage/Dropbox", "Reports"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(home, "Reports/link-to-icloud")
	if err := os.Symlink(filepath.Join(home, "Library/Mobile Documents"), link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		filepath.Join(home, "Reports/groundskeeper"):                           "",
		filepath.Join(home, "Documents/gk"):                                    "Documents, which iCloud can sync",
		filepath.Join(home, "Desktop"):                                         "Desktop, which iCloud can sync",
		filepath.Join(home, "Library/Mobile Documents/com~apple~CloudDocs/gk"): "iCloud Drive",
		filepath.Join(home, "Library/CloudStorage/Dropbox/reports"):            "a cloud storage folder",
		filepath.Join(link, "not-yet-created"):                                 "iCloud Drive",
		filepath.Join(home, "Documents-old/gk"):                                "",
	}
	for path, want := range cases {
		if got := SyncedFolder(path, home); got != want {
			t.Errorf("SyncedFolder(%s) = %q, want %q", strings.TrimPrefix(path, home), got, want)
		}
	}
}

func TestFullDiskAccess(t *testing.T) {
	e := env(t, fake{})
	safari := filepath.Join(e.Home, "Library/Safari")
	if err := os.MkdirAll(safari, 0o700); err != nil {
		t.Fatal(err)
	}
	if c := fullDiskAccess(e); c.Status != OK {
		t.Errorf("readable = %+v", c)
	}
	os.Chmod(safari, 0)
	t.Cleanup(func() { os.Chmod(safari, 0o700) })
	if c := fullDiskAccess(e); c.Status != Warn || !strings.Contains(c.Fix, "bin/groundskeeper") {
		t.Errorf("blocked = %+v", c)
	}
}

func TestLogFilesMustBePrivate(t *testing.T) {
	e := env(t, fake{})
	logs := filepath.Join(e.Home, "Library/Logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	if c := logFiles(e); c.Status != OK {
		t.Errorf("no logs yet = %+v", c)
	}
	main := filepath.Join(logs, "groundskeeper.log")
	launchd := filepath.Join(logs, "groundskeeper.launchd.log")
	os.WriteFile(main, nil, 0o600)
	os.WriteFile(launchd, nil, 0o644)
	os.Chmod(launchd, 0o644)

	c := logFiles(e)
	if c.Status != Warn || !strings.Contains(c.Detail, "groundskeeper.launchd.log") || !strings.Contains(c.Fix, "chmod 600") {
		t.Fatalf("readable launchd log = %+v", c)
	}
	if strings.Contains(c.Detail, "groundskeeper.log ") {
		t.Errorf("private log reported: %+v", c)
	}
	os.Chmod(launchd, 0o600)
	if c := logFiles(e); c.Status != OK {
		t.Errorf("private logs = %+v", c)
	}
}

func TestPhotosHelper(t *testing.T) {
	e := env(t, fake{})
	if c := photosHelper(e); c.Status != Warn || !strings.Contains(c.Detail, "missing") || !strings.Contains(c.Fix, "make install") {
		t.Errorf("missing = %+v", c)
	}
	path := HelperPath(e.Home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if c := photosHelper(e); c.Status != Warn || !strings.Contains(c.Detail, "executable") {
		t.Errorf("not executable = %+v", c)
	}
	os.Chmod(path, 0o755)
	if c := photosHelper(e); c.Status != OK || c.Detail != path {
		t.Errorf("installed = %+v", c)
	}
}

func TestPhotosAccess(t *testing.T) {
	cases := map[string]string{
		"":               "couldn't check",
		"authorized":     "readable",
		"limited":        "limited",
		"denied":         "denied",
		"restricted":     "restricted",
		"not-determined": "not granted yet",
	}
	for access, detail := range cases {
		c := photosAccess(context.Background(), env(t, fake{access: access}))
		wantOK := access == "authorized"
		if (c.Status == OK) != wantOK || !strings.Contains(c.Detail, detail) || (c.Fix == "") != wantOK {
			t.Errorf("%q: got %+v", access, c)
		}
	}
}
