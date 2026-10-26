package config

import (
	"os"
	"path/filepath"
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
