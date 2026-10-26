package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

func TestToolchainsFindsVersionsWithHints(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".rustup/toolchains/stable-aarch64-apple-darwin/bin/rustc"), 10_000)
	writeFile(t, filepath.Join(home, ".volta/tools/image/node/24.12.0/bin/node"), 10_000)
	writeFile(t, filepath.Join(home, ".pyenv/versions/3.12.12/bin/python"), 10_000)
	uv := filepath.Join(home, ".local/share/uv/python")
	writeFile(t, filepath.Join(uv, "cpython-3.14.4-macos-aarch64-none/bin/python"), 10_000)
	writeFile(t, filepath.Join(uv, ".temp/x"), 10_000)
	if err := os.Symlink(filepath.Join(uv, "cpython-3.14.4-macos-aarch64-none"), filepath.Join(uv, "cpython-3.14-macos-aarch64-none")); err != nil {
		t.Fatal(err)
	}

	f, err := Toolchains{Size: size.New(time.Minute, 2), MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	items := byLabel(f.Items)
	want := map[string]string{
		"rustup: stable-aarch64-apple-darwin":   "rustup toolchain uninstall 'stable-aarch64-apple-darwin'",
		"Volta: Node 24.12.0":                   "rm -rf " + ShellQuote(filepath.Join(home, ".volta/tools/image/node/24.12.0")),
		"pyenv: Python 3.12.12":                 "pyenv uninstall '3.12.12'",
		"uv: cpython-3.14.4-macos-aarch64-none": "uv python uninstall 'cpython-3.14.4-macos-aarch64-none'",
	}
	if len(items) != len(want) {
		t.Fatalf("got %v, want %d items (no symlink alias, no dot folder)", keys(items), len(want))
	}
	for label, hint := range want {
		it, ok := items[label]
		if !ok || it.Hint != hint || it.Safety != Review {
			t.Errorf("%s = %+v, want hint %q", label, it, hint)
		}
	}
}

func TestToolchainsHintQuotesCraftedName(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".pyenv/versions/x; rm -rf ~/bin/python"), 10_000)

	f, err := Toolchains{Size: size.New(time.Minute, 2), MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 1 || f.Items[0].Hint != "pyenv uninstall 'x; rm -rf ~'" {
		t.Fatalf("got %+v", f.Items)
	}
}
