package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinSizeMB != 100 || cfg.LogLevel != "info" {
		t.Fatalf("got %+v, want defaults", cfg)
	}
}

func TestLoadOverridesOnlyGivenFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"minSizeMB": 5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinSizeMB != 5 || cfg.OutputDir != "~/Reports/groundskeeper" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestExpand(t *testing.T) {
	cases := map[string]string{
		"~":        "/h",
		"~/a/b":    "/h/a/b",
		"/abs":     "/abs",
		"~other/x": "~other/x",
	}
	for in, want := range cases {
		if got := Expand(in, "/h"); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetPhotosSourceKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"minSizeMB": 5, "futureKey": [1], "photosSource": "device"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetPhotosSource(path, "mac-library"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PhotosSource != "mac-library" || cfg.MinSizeMB != 5 {
		t.Fatalf("got %+v", cfg)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"futureKey": [`) {
		t.Errorf("unknown key lost: %s", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
}

func TestSetPhotosSourceCreatesPrivateFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "groundskeeper")
	path := filepath.Join(dir, "config.json")
	if err := SetPhotosSource(path, "device"); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(path); err != nil || cfg.PhotosSource != "device" || cfg.MinSizeMB != 100 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v, want 0700", info.Mode().Perm())
	}
}

func TestSetPhotosSourceLeavesInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetPhotosSource(path, "device"); err == nil {
		t.Fatal("want error for invalid JSON")
	}
	if data, _ := os.ReadFile(path); string(data) != "{" {
		t.Errorf("file changed to %q", data)
	}
}
