package tui

import (
	"fmt"
	"image/jpeg"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// Thumbnail width in cells: at most thumbCols, shrunk to fit the window,
// and not shown below minThumbCols.
const (
	thumbCols    = 30
	minThumbCols = 12
)

// thumbSize returns the cells for each of n thumbnails side by side in a
// window width cells wide, two apart. Kitty fits each image into its cells
// keeping its aspect ratio; with cells about twice as tall as wide, rows of
// two thirds the columns fit a 3:4 portrait.
func thumbSize(n, width int) (cols, rows int) {
	cols = min(thumbCols, width/n-2)
	return cols, cols * 2 / 3
}

// sendImage returns the Kitty graphics command that sends the JPEG at path
// as image id, as PNG since the protocol doesn't take JPEG. It isn't shown
// until placeImage places it.
func sendImage(id int, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = kitty.EncodeGraphics(&b, img, &kitty.Options{
		Action:       kitty.Transmit,
		Transmission: kitty.Direct, // the zero value sends no image data
		Format:       kitty.PNG,
		ID:           id,
		Quiet:        2, // no replies, which would arrive as input
		Chunk:        true,
	})
	return b.String(), err
}

// placeImage returns the command that places image id in cols by rows cells
// for Unicode placeholders to show. It always uses placement 1, so placing
// an image again at another size replaces the earlier placement.
func placeImage(id, cols, rows int) string {
	o := kitty.Options{Action: kitty.Put, ID: id, PlacementID: 1, VirtualPlacement: true, Columns: cols, Rows: rows, Quiet: 2}
	return ansi.KittyGraphics(nil, o.Options()...)
}

// placeholders returns rows lines of cols placeholder cells showing image
// id, which must be below 256 to fit the 256-color foreground.
func placeholders(id, cols, rows int) []string {
	lines := make([]string, rows)
	for r := range rows {
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[39m")
		lines[r] = b.String()
	}
	return lines
}

// deleteImage returns the command that deletes image id and frees its data.
func deleteImage(id int) string {
	return ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", id), "q=2")
}
