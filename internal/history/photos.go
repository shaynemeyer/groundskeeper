package history

import (
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

// ErrNoPhotoScan means gk iphone hasn't saved a photo scan yet.
var ErrNoPhotoScan = errors.New("no photo scan has been saved")

// PhotoScan is the last photo scan, kept for the review.
type PhotoScan struct {
	Time   time.Time     `json:"time"`
	Photos iphone.Photos `json:"photos"`
}

// PhotoScanPath is where the last photo scan is saved. Its name doesn't
// match the snapshot pattern, so pruning leaves it alone.
func PhotoScanPath(home string) string {
	return filepath.Join(Dir(home), "photo-scan.json")
}

// SavePhotoScan writes s to path, replacing the previous scan.
func SavePhotoScan(path string, s PhotoScan) error {
	return SaveJSON(path, s)
}

// LoadPhotoScan reads the last photo scan. It returns ErrNoPhotoScan when
// there is none.
func LoadPhotoScan(path string) (PhotoScan, error) {
	var s PhotoScan
	err := load(path, &s)
	if errors.Is(err, fs.ErrNotExist) {
		return s, ErrNoPhotoScan
	}
	return s, err
}
