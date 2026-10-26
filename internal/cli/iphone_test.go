package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
