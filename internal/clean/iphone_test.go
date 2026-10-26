package clean

import (
	"errors"
	"slices"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
)

var phone = iphone.Phone{Apps: []iphone.App{
	{BundleID: "com.example.app1", Name: "App One", AppBytes: 100, DataBytes: 900},
	{BundleID: "com.example.app2", Name: "App Two", AppBytes: 50, DataBytes: 50},
	{BundleID: "com.apple.Pages", Name: "Pages", AppBytes: 400},
}}

// fakePhone records uninstalls without touching a device.
type fakePhone struct {
	removed []string
	failOn  string
}

func (p *fakePhone) uninstall(id string) error {
	if id == p.failOn {
		return errors.New("received uninstall error")
	}
	p.removed = append(p.removed, id)
	return nil
}

func iphoneApps(p *fakePhone, apply bool, ids ...string) IPhoneApps {
	return IPhoneApps{
		IDs:       ids,
		Snapshot:  phone,
		Keep:      []string{"com.example.app2"},
		Apply:     apply,
		Confirm:   func(iphone.App) bool { return true },
		Uninstall: p.uninstall,
		Log:       logging.Discard(),
	}
}

func TestIPhoneAppsRefuses(t *testing.T) {
	cases := map[string]string{
		"com.example.missing":   "not in the last iPhone scan; run gk iphone first",
		"com.apple.Pages":       "one of Apple's apps",
		"com.apple.Preferences": "not in the last iPhone scan; run gk iphone first",
		"com.example.app2":      "listed in iphoneKeep",
	}
	for id, want := range cases {
		p := &fakePhone{}
		got := iphoneApps(p, true, id).Clean()
		if got[0].Outcome != Refused || got[0].Reason != want {
			t.Errorf("%s: got %q %q, want refused %q", id, got[0].Outcome, got[0].Reason, want)
		}
		if len(p.removed) > 0 {
			t.Errorf("%s: uninstalled %v", id, p.removed)
		}
	}
}

func TestIPhoneAppsDryRun(t *testing.T) {
	p := &fakePhone{}
	c := iphoneApps(p, false, "com.example.app1")
	c.Confirm = func(iphone.App) bool { t.Error("dry run asked to confirm"); return false }
	got := c.Clean()
	if got[0].Outcome != WouldUninstall || got[0].App.Bytes() != 1000 {
		t.Errorf("got %+v", got[0])
	}
	if len(p.removed) > 0 {
		t.Errorf("dry run uninstalled %v", p.removed)
	}
}

func TestIPhoneAppsApply(t *testing.T) {
	p := &fakePhone{}
	got := iphoneApps(p, true, "com.example.app1").Clean()
	if got[0].Outcome != Uninstalled {
		t.Errorf("got %+v", got[0])
	}
	if !slices.Equal(p.removed, []string{"com.example.app1"}) {
		t.Errorf("removed %v", p.removed)
	}
}

func TestIPhoneAppsDeclined(t *testing.T) {
	p := &fakePhone{}
	c := iphoneApps(p, true, "com.example.app1")
	c.Confirm = func(iphone.App) bool { return false }
	if got := c.Clean(); got[0].Outcome != Declined {
		t.Errorf("got %+v", got[0])
	}
	if len(p.removed) > 0 {
		t.Errorf("uninstalled without confirmation: %v", p.removed)
	}
}

func TestIPhoneAppsFailed(t *testing.T) {
	p := &fakePhone{failOn: "com.example.app1"}
	if got := iphoneApps(p, true, "com.example.app1").Clean(); got[0].Outcome != Failed {
		t.Errorf("got %+v", got[0])
	}
}
