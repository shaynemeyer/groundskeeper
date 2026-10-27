package main

import (
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/shaynemeyer/groundskeeper"

// usbPackage is the only package allowed to import go-ios, which talks to
// the iPhone over USB through usbmuxd.
const usbPackage = module + "/internal/iphone"

// networkImport reports whether an import can open network connections.
func networkImport(path string) bool {
	return path == "net" || strings.HasPrefix(path, "net/") ||
		path == "crypto/tls" || strings.HasPrefix(path, "golang.org/x/net")
}

// TestNoNetworkImports keeps the tool offline: none of its own packages may
// import a network package, and only usbPackage may import go-ios.
func TestNoNetworkImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", module+"/...").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		pkg, imports := fields[0], fields[1:]
		for _, imp := range imports {
			if networkImport(imp) {
				t.Errorf("%s imports %s", pkg, imp)
			}
			if strings.HasPrefix(imp, "github.com/danielpaulus/go-ios") && pkg != usbPackage {
				t.Errorf("%s imports %s; only %s may talk to the iPhone", pkg, imp, usbPackage)
			}
		}
	}
}
