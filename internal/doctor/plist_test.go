package doctor

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// installedPlist fills in the repo's template the way make install does.
func installedPlist(t *testing.T, home string) string {
	t.Helper()
	tmpl, err := os.ReadFile("../../launchd/" + Label + ".plist")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tmpl), "/Users/") {
		t.Fatal("template has a hardcoded home folder")
	}
	path := filepath.Join(t.TempDir(), Label+".plist")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(tmpl), "__HOME__", home)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func realRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func TestPlistTemplateIsValidAndRunsReadOnlyScan(t *testing.T) {
	path := installedPlist(t, "/Users/someone")
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint: %v\n%s", err, out)
	}
	out, err := realRun(context.Background(), "plutil", "-convert", "json", "-o", "-", path)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Label            string
		ProgramArguments []string
		ProcessType      string
		StandardOutPath  string
	}
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	want := []string{"/Users/someone/bin/groundskeeper", "scan", "--notify"}
	if p.Label != Label || !slices.Equal(p.ProgramArguments, want) || p.ProcessType != "" ||
		p.StandardOutPath != "/Users/someone/Library/Logs/groundskeeper.launchd.log" {
		t.Fatalf("plist = %+v", p)
	}
}

func TestNextRunFromRealTemplate(t *testing.T) {
	e := Env{Run: realRun, Now: sunday}
	next, ok := nextRun(context.Background(), e, installedPlist(t, "/Users/someone"))
	if !ok || !next.Equal(time.Date(2026, 10, 26, 9, 0, 0, 0, time.Local)) {
		t.Fatalf("next = %v %v, want Mon 26 Oct 09:00", next, ok)
	}
}
