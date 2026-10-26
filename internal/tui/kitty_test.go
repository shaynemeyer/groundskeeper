package tui

import (
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// writeJPEG writes a small JPEG into a temporary folder and returns its path.
func writeJPEG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "thumb.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 6, 8)), nil); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSendImage(t *testing.T) {
	seq, err := sendImage(7, writeJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"f=100", "i=7", "q=2"} {
		if !strings.Contains(seq, want) {
			t.Errorf("command lacks %s: %.80q", want, seq)
		}
	}
	if strings.Contains(seq, "a=") || strings.Contains(seq, "U=1") {
		t.Errorf("sending should only transmit, the default action: %.80q", seq)
	}
	if len(seq) < 100 {
		t.Errorf("command carries no image data: %q", seq)
	}
}

func TestSendImageNotJPEG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumb.jpg")
	if err := os.WriteFile(path, []byte("not a jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sendImage(1, path); err == nil {
		t.Error("sendImage accepted a file that isn't a JPEG")
	}
}

func TestPlaceholders(t *testing.T) {
	lines := placeholders(42, 3, 2)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	p, d := string(kitty.Placeholder), kitty.Diacritic
	want := "\x1b[38;5;42m" + p + string(d(1)) + string(d(0)) + p + string(d(1)) + string(d(1)) + p + string(d(1)) + string(d(2)) + "\x1b[39m"
	if lines[1] != want {
		t.Errorf("second row = %q, want %q", lines[1], want)
	}
	if w := ansi.StringWidth(lines[0]); w != 3 {
		t.Errorf("row is %d cells wide, want 3", w)
	}
}

func TestDeleteImage(t *testing.T) {
	if got, want := deleteImage(5), "\x1b_Ga=d,d=I,i=5,q=2\x1b\\"; got != want {
		t.Errorf("deleteImage(5) = %q, want %q", got, want)
	}
}

func TestPlaceImage(t *testing.T) {
	if got, want := placeImage(3, 27, 18), "\x1b_Gq=2,i=3,p=1,U=1,c=27,r=18,a=p\x1b\\"; got != want {
		t.Errorf("placeImage = %q, want %q", got, want)
	}
}

func TestThumbSize(t *testing.T) {
	for _, c := range []struct{ n, width, cols, rows int }{
		{2, 120, 30, 20}, // room to spare: full size
		{3, 88, 27, 18},  // shrunk to fit
		{3, 30, 8, 5},    // below minThumbCols
	} {
		if cols, rows := thumbSize(c.n, c.width); cols != c.cols || rows != c.rows {
			t.Errorf("thumbSize(%d, %d) = %d, %d, want %d, %d", c.n, c.width, cols, rows, c.cols, c.rows)
		}
	}
}
