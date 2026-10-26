package history

import (
	"os"
	"testing"
	"time"
)

func TestPhotoScanSurvivesPruning(t *testing.T) {
	home := t.TempDir()
	path := PhotoScanPath(home)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := SavePhotoScan(path, PhotoScan{Time: old}); err != nil {
		t.Fatal(err)
	}
	if _, err := Prune(Dir(home), old.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("pruning removed the photo scan: %v", err)
	}
	if scans, _ := Scans(Dir(home)); len(scans) != 0 {
		t.Errorf("photo scan read as a scan snapshot: %+v", scans)
	}
}
