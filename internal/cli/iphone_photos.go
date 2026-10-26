package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/metrics"
)

// photoRunner runs photos-helper. Tests replace it.
var photoRunner iphone.HelperRunner = iphone.RunHelper

// photoVideos is how many of the largest videos the report lists.
const photoVideos = 10

// photosSource returns the photosSource to scan. The first time, it asks in
// the terminal and saves the answer unless dry. When photos can't be scanned
// it returns "" and a note saying why.
func photosSource(cmd *cobra.Command, global *globalOptions, home string, cfg config.Config, dry bool) (string, string, error) {
	switch cfg.PhotosSource {
	case iphone.SourceDevice, iphone.SourceMacLibrary:
		return cfg.PhotosSource, "", nil
	case "":
	default:
		return "", fmt.Sprintf("Photos weren't scanned: photosSource %q isn't device or mac-library. Run gk setup photos.", cfg.PhotosSource), nil
	}
	source, err := photosAsker(cmd.OutOrStdout())
	switch {
	case errors.Is(err, errNoTerminal):
		return "", "Photos weren't scanned: the iCloud Photos question needs a terminal. Run gk setup photos.", nil
	case err != nil:
		return "", "", err
	case source == "":
		return "", "Photos weren't scanned: the iCloud Photos question was cancelled. Run gk setup photos.", nil
	}
	if !dry {
		path := configPath(global, home)
		if err := config.SetPhotosSource(path, source); err != nil {
			return "", "", fmt.Errorf("saving photosSource to %s: %w", path, err)
		}
	}
	return source, "", nil
}

// scanPhotos runs the photo scan, timed as a scanner for Run details. A
// failed scan gives a note instead of photos, so the rest of the report is
// still written.
func scanPhotos(ctx context.Context, log *logging.Logger, home string, cfg config.Config, source string, now time.Time) (*iphone.Photos, metrics.Scanner, string) {
	begin := time.Now()
	h := iphone.Helper{Path: doctor.HelperPath(home), Source: source, Run: photoRunner}
	window := time.Duration(cfg.PhotoWindowMinutes) * time.Minute
	p, err := iphone.ScanPhotos(ctx, h, now, window, cfg.ScreenshotDays, photoVideos)
	m := metrics.Scanner{Name: "iPhone photos", DurationMS: time.Since(begin).Milliseconds()}
	if err != nil {
		m.Error = photoKind(err)
		log.Error("photo scan failed", logging.ID("source", source), logging.ID("error_kind", m.Error))
		return nil, m, photoNote(err)
	}
	m.Items = len(p.Assets)
	m.Skipped = len(p.Duplicates.Failed)
	log.Info("photos scanned", logging.ID("source", source), logging.Duration(time.Since(begin)),
		logging.Count("assets", len(p.Assets)), logging.Count("groups", len(p.Duplicates.Groups)),
		logging.Count("screenshots", len(p.OldScreenshots)), logging.Count("failed", len(p.Duplicates.Failed)))
	return &p, m, ""
}

// photoNote says why the photo scan failed and what to do about it.
func photoNote(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "Photos weren't scanned: photos-helper isn't installed. Run make install."
	case errors.Is(err, iphone.ErrNoDevice):
		return "Photos weren't scanned: no iPhone answered. Unlock it and run gk iphone again."
	case errors.Is(err, iphone.ErrLocked):
		return "Photos weren't scanned: the iPhone is locked. Unlock it and run gk iphone again."
	case errors.Is(err, iphone.ErrDenied):
		return "Photos weren't scanned: this terminal app doesn't have Photos access. See gk doctor."
	case errors.Is(err, iphone.ErrTimeout):
		return "Photos weren't scanned: the iPhone didn't send its photos in time. Unplug it, plug it in again and run gk iphone again."
	}
	return "Photos weren't scanned: photos-helper failed. Run gk doctor."
}

func photoKind(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "no_helper"
	case errors.Is(err, iphone.ErrNoDevice):
		return "no_device"
	case errors.Is(err, iphone.ErrLocked):
		return "locked"
	case errors.Is(err, iphone.ErrDenied):
		return "denied"
	case errors.Is(err, iphone.ErrTimeout):
		return "timeout"
	}
	return "helper"
}
