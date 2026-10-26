// Package config loads ~/.config/groundskeeper/config.json over built-in defaults.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config holds user settings. Paths may start with "~".
type Config struct {
	ProjectRoots       []string `json:"projectRoots"`
	StaleDays          int      `json:"staleDays"`
	MinSizeMB          int64    `json:"minSizeMB"`
	OutputDir          string   `json:"outputDir"`
	ExtraPaths         []string `json:"extraPaths"`
	ExcludePaths       []string `json:"excludePaths"`
	AppUnusedDays      int      `json:"appUnusedDays"`
	AppExclude         []string `json:"appExclude"`
	IphoneUnusedDays   int      `json:"iphoneUnusedDays"`
	IphoneKeep         []string `json:"iphoneKeep"`
	PhotosSource       string   `json:"photosSource"`
	PhotoWindowMinutes int      `json:"photoWindowMinutes"`
	ScreenshotDays     int      `json:"screenshotDays"`
	LogLevel           string   `json:"logLevel"`
}

// Default returns the built-in settings.
func Default() Config {
	return Config{
		ProjectRoots:       []string{"~/projects", "~/github", "~/code", "~/Developer"},
		StaleDays:          90,
		MinSizeMB:          100,
		OutputDir:          "~/Reports/groundskeeper",
		AppUnusedDays:      120,
		IphoneUnusedDays:   120,
		PhotoWindowMinutes: 10,
		ScreenshotDays:     30,
		LogLevel:           "info",
	}
}

// DefaultPath returns the config file location under home.
func DefaultPath(home string) string {
	return filepath.Join(home, ".config", "groundskeeper", "config.json")
}

// Load reads path over the defaults. A missing file returns the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

// Expand replaces a leading "~" with home.
func Expand(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}
