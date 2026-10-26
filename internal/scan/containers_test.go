package scan

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/shaynemeyer/groundskeeper/internal/size"
)

const podmanJSON = `[
 {"Type":"Images","Total":141,"Active":1,"RawSize":8243454365,"RawReclaimable":8233308548},
 {"Type":"Containers","Total":1,"Active":0,"RawSize":679061369,"RawReclaimable":679061369},
 {"Type":"Local Volumes","Total":26,"Active":1,"RawSize":2353462098,"RawReclaimable":1602417930}
]`

func scanContainers(t *testing.T, home string, run Runner) Findings {
	t.Helper()
	f, err := Containers{Size: size.New(time.Minute, 2), Run: run, MinBytes: 1}.Scan(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestContainersFromPodmanSystemDF(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(podmanJSON), nil
	}
	items := byLabel(scanContainers(t, t.TempDir(), run).Items)

	if !slices.Equal(gotArgs, []string{"podman", "system", "df", "--format", "json"}) {
		t.Errorf("ran %v", gotArgs)
	}
	images := items["Podman Images (reclaimable)"]
	if images.Bytes != 8233308548 || images.Hint != "podman image prune -a" || images.Note != "141 images, 1 in use" {
		t.Errorf("images = %+v", images)
	}
	if v := items["Podman Local Volumes (reclaimable)"]; v.Hint != "podman volume prune" || v.Safety != Review {
		t.Errorf("volumes = %+v", v)
	}
}

func TestContainersMeasuresMachineImage(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".local/share/containers/podman/machine/applehv/disk.raw"), 100_000)
	noPodman := func(context.Context, string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }

	f := scanContainers(t, home, noPodman)
	if len(f.Items) != 1 || f.Items[0].Label != "Podman machine disk image" || f.Items[0].Bytes < 100_000 {
		t.Fatalf("got %+v", f.Items)
	}
}

func TestContainersStoppedMachineReportsNothing(t *testing.T) {
	stopped := func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("cannot connect to Podman")
	}
	if f := scanContainers(t, t.TempDir(), stopped); len(f.Items) != 0 {
		t.Fatalf("got %+v", f.Items)
	}
}

func TestContainersIgnoresBadJSON(t *testing.T) {
	bad := func(context.Context, string, ...string) ([]byte, error) { return []byte("Error: oops"), nil }
	if f := scanContainers(t, t.TempDir(), bad); len(f.Items) != 0 {
		t.Fatalf("got %+v", f.Items)
	}
}
