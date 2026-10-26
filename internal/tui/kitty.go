package tui

import (
	"fmt"
	"image/jpeg"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// Thumbnail size in cells. Kitty fits each image into it, keeping its
// aspect ratio; a 3:4 portrait fills it in cells about twice as tall as wide.
const (
	thumbCols = 30
	thumbRows = 20
)

// sendImage returns the Kitty graphics command that sends the JPEG at path
// as image id, as PNG since the protocol doesn't take JPEG, with a virtual
// placement of cols by rows cells for Unicode placeholders to show.
func sendImage(id int, path string, cols, rows int) (string, error) {
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
		Action:           kitty.TransmitAndPut,
		Transmission:     kitty.Direct, // the zero value sends no image data
		Format:           kitty.PNG,
		ID:               id,
		VirtualPlacement: true,
		Columns:          cols,
		Rows:             rows,
		Quiet:            2, // no replies, which would arrive as input
		Chunk:            true,
	})
	return b.String(), err
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
