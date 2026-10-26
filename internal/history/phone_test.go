package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

func TestSaveAndLoadPhonesPrivately(t *testing.T) {
	home := t.TempDir()
	dir := PhoneDir(home)
	app := iphone.App{BundleID: "com.example.app1", Name: "App 1", AppBytes: 10, DataBytes: 5}
	for _, s := range []PhoneSnapshot{
		{Time: at(26, 9), Phone: iphone.Phone{IOS: "27.0.1", Apps: []iphone.App{app}}},
		{Time: at(22, 9)},
	} {
		if err := SavePhone(dir, s); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Phones(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[1].Time.Equal(at(26, 9)) || got[1].Phone.Apps[0] != app {
		t.Fatalf("got %+v", got)
	}
	for _, p := range []string{dir, filepath.Join(dir, "2026-10-26T0900.json")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if want := os.FileMode(0o600); !info.IsDir() && info.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", p, info.Mode().Perm(), want)
		}
		if want := os.FileMode(0o700); info.IsDir() && info.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", p, info.Mode().Perm(), want)
		}
	}

	scans, err := Scans(Dir(home))
	if err != nil || len(scans) != 0 {
		t.Errorf("Mac scans picked up iPhone snapshots: %+v, %v", scans, err)
	}
}
