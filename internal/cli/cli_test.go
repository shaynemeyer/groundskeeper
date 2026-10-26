package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const marker = "GK-SECRET-MARKER-7f3a"

func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NO_COLOR", "")
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
		if c.Short == "" || c.Long == "" || c.Example == "" {
			t.Errorf("%q needs Short, Long and Example", c.CommandPath())
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
