package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/clean"
	"github.com/shaynemeyer/groundskeeper/internal/history"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

// fakePhone makes phoneReader return the recorded fixture, or err.
func fakePhone(t *testing.T, err error) {
	t.Helper()
	b, readErr := os.ReadFile("../iphone/testdata/phone.json")
	if readErr != nil {
		t.Fatal(readErr)
	}
	var raw iphone.Raw
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	phoneReader = func() (iphone.Raw, error) { return raw, err }
	t.Cleanup(func() { phoneReader = iphone.Read })
}

func TestIPhoneAddsPhoneToReport(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	out := run(t, "iphone")

	if !strings.Contains(out, "iPhone       73.0 GB free of 256.0 GB") {
		t.Errorf("summary = %q", out)
	}
	html, err := os.ReadFile(filepath.Join(home, "Reports/groundskeeper/latest.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"iOS 27.0.1", "Shared app files", "npm"} {
		if !strings.Contains(string(html), want) {
			t.Errorf("report missing %q", want)
		}
	}
	phones, err := history.Phones(history.PhoneDir(home))
	if err != nil || len(phones) != 1 || len(phones[0].Phone.Apps) != 154 {
		t.Fatalf("iPhone snapshots: %d, %v", len(phones), err)
	}
	scans := readSnapshots(t, home)
	if len(scans) != 1 || !slices.ContainsFunc(scans[0].Run.Scanners, func(s metrics.Scanner) bool { return s.Name == "iPhone" }) {
		t.Errorf("snapshot run details lack the iPhone read: %+v", scans)
	}
}

func TestIPhoneNotConnectedWritesNothing(t *testing.T) {
	for name, err := range map[string]error{
		"connect":    iphone.ErrNoDevice,
		"Trust":      iphone.ErrNotTrusted,
		"plug it in": iphone.ErrRead,
	} {
		t.Run(name, func(t *testing.T) {
			home := fakeHome(t)
			fakePhone(t, err)
			var out bytes.Buffer
			root := NewRoot()
			root.SetArgs([]string{"iphone"})
			root.SetOut(&out)
			root.SetErr(&out)
			got := root.Execute()
			if got == nil || !strings.Contains(got.Error(), name) {
				t.Fatalf("error = %v, want one mentioning %q", got, name)
			}
			for _, p := range []string{"Reports", ".local/state/groundskeeper"} {
				if _, err := os.Stat(filepath.Join(home, p)); !os.IsNotExist(err) {
					t.Errorf("%s exists after a failed read", p)
				}
			}
		})
	}
}

func TestIPhoneDrySavesNothing(t *testing.T) {
	home := fakeHome(t)
	fakePhone(t, nil)
	out := run(t, "iphone", "--dry")
	if !strings.Contains(out, "iPhone") || !strings.Contains(out, "Dry run") {
		t.Errorf("summary = %q", out)
	}
	if _, err := os.Stat(stateDir(home)); !os.IsNotExist(err) {
		t.Error("state directory exists after --dry")
	}
}

// fakeUninstaller records uninstalls instead of reaching a phone.
func fakeUninstaller(t *testing.T) *[]string {
	t.Helper()
	var removed []string
	phoneUninstaller = func(id string) error { removed = append(removed, id); return nil }
	t.Cleanup(func() { phoneUninstaller = iphone.Uninstall })
	return &removed
}

// runWithInput runs the CLI with stdin set to input.
func runWithInput(t *testing.T, input string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRoot()
	root.SetArgs(args)
	root.SetIn(strings.NewReader(input))
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestCleanIPhoneAppsNeedsAScan(t *testing.T) {
	fakeHome(t)
	removed := fakeUninstaller(t)
	root := NewRoot()
	root.SetArgs([]string{"clean", "iphone-apps", "com.example.app1", "--apply"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "run gk iphone first") {
		t.Fatalf("error = %v", err)
	}
	if len(*removed) > 0 {
		t.Errorf("uninstalled %v", *removed)
	}
}

func TestCleanIPhoneAppsDryRunByDefault(t *testing.T) {
	fakeHome(t)
	fakePhone(t, nil)
	run(t, "iphone")
	removed := fakeUninstaller(t)
	out := runWithInput(t, "y\n", "clean", "iphone-apps", "com.example.app1")
	if !strings.Contains(out, clean.WouldUninstall) || !strings.Contains(out, "Dry run") || strings.Contains(out, "[y/N]") {
		t.Errorf("output = %q", out)
	}
	if len(*removed) > 0 {
		t.Errorf("dry run uninstalled %v", *removed)
	}
}

func TestCleanIPhoneAppsApplyAsksFirst(t *testing.T) {
	fakeHome(t)
	fakePhone(t, nil)
	run(t, "iphone")
	removed := fakeUninstaller(t)
	out := runWithInput(t, "n\ny\n", "clean", "iphone-apps", "com.example.app1", "com.example.app2", "com.apple.Pages", "--apply")
	if !slices.Equal(*removed, []string{"com.example.app2"}) {
		t.Errorf("uninstalled %v, want only com.example.app2", *removed)
	}
	for _, want := range []string{"deleted permanently", "offload", clean.Declined, clean.Uninstalled, "one of Apple's apps"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestCleanIPhoneAppsNoAnswerKeepsApp(t *testing.T) {
	fakeHome(t)
	fakePhone(t, nil)
	run(t, "iphone")
	removed := fakeUninstaller(t)
	runWithInput(t, "", "clean", "iphone-apps", "com.example.app1", "--apply")
	if len(*removed) > 0 {
		t.Errorf("uninstalled %v with no answer", *removed)
	}
}

func TestCleanIPhoneAppsCompletesSnapshotIDs(t *testing.T) {
	fakeHome(t)
	fakePhone(t, nil)
	run(t, "iphone")
	out := run(t, "__complete", "clean", "iphone-apps", "")
	if !strings.Contains(out, "com.example.app1\t") || strings.Contains(out, "com.apple.") {
		t.Errorf("completions = %q", out)
	}
}
