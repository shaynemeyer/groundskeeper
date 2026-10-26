package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

const podmanTimeout = 30 * time.Second

type podmanDF struct {
	Type           string `json:"Type"`
	Total          int    `json:"Total"`
	Active         int    `json:"Active"`
	RawSize        int64  `json:"RawSize"`
	RawReclaimable int64  `json:"RawReclaimable"`
}

var podmanHints = map[string]struct{ hint, note string }{
	"Images":        {"podman image prune -a", "%d images, %d in use"},
	"Containers":    {"podman container prune", "%d containers, %d running"},
	"Local Volumes": {"podman volume prune", "%d volumes, %d in use; named volumes may hold data, add -a to include them"},
}

// Containers reports what Podman could reclaim inside its machine, and how
// much disk the machine's image file uses on the Mac. A stopped machine is
// never started; it just reports nothing.
type Containers struct {
	Size     *size.Measurer
	Run      Runner // nil runs the real podman
	MinBytes int64
}

// Name returns the scanner name.
func (c Containers) Name() string { return "Containers" }

// Scan asks podman system df for reclaimable space and measures the VM image.
func (c Containers) Scan(ctx context.Context, home string) (Findings, error) {
	run := c.Run
	if run == nil {
		run = runTool
	}
	items := []Item{{
		Section: "Containers",
		Label:   "Podman machine disk image",
		Path:    filepath.Join(home, ".local/share/containers/podman/machine"),
		Safety:  Review,
		Note:    "disk the Podman VM uses on this Mac; the rows below are inside it",
	}}
	skipped := measure(ctx, c.Size, items)

	ctx, cancel := context.WithTimeout(ctx, podmanTimeout)
	defer cancel()
	if out, err := run(ctx, "podman", "system", "df", "--format", "json"); err == nil {
		items = append(items, podmanItems(out)...)
	}
	return Findings{Items: keep(items, c.MinBytes), Skipped: skipped}, nil
}

func podmanItems(out []byte) []Item {
	var rows []podmanDF
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil
	}
	var items []Item
	for _, r := range rows {
		h, ok := podmanHints[r.Type]
		if !ok {
			continue
		}
		items = append(items, Item{
			Section: "Containers",
			Label:   "Podman " + r.Type + " (reclaimable)",
			Bytes:   r.RawReclaimable,
			Safety:  Review,
			Hint:    h.hint,
			Note:    fmt.Sprintf(h.note, r.Total, r.Active),
		})
	}
	return items
}
