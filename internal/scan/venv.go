package scan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// brokenVenv reports whether a venv's pyvenv.cfg points at a Python folder
// that no longer exists, and returns that folder.
func brokenVenv(venv string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(venv, "pyvenv.cfg"))
	if err != nil {
		return "", false
	}
	for line := range strings.Lines(string(data)) {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "home" {
			continue
		}
		home := strings.TrimSpace(value)
		_, err := os.Stat(home)
		return home, errors.Is(err, fs.ErrNotExist)
	}
	return "", false
}
