package iphone

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func loadFixture(t *testing.T) Raw {
	t.Helper()
	b, err := os.ReadFile("testdata/phone.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw Raw
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseStorage(t *testing.T) {
	p := Parse(loadFixture(t))
	if p.IOS != "27.0.1" || p.TotalBytes != 256_000_000_000 || p.FreeBytes != 72_995_266_560 {
		t.Errorf("got iOS %q, total %d, free %d", p.IOS, p.TotalBytes, p.FreeBytes)
	}
}

func TestParseApps(t *testing.T) {
	apps := Parse(loadFixture(t)).Apps
	if len(apps) != 154 {
		t.Fatalf("got %d apps, want 154", len(apps))
	}
	if !slices.IsSortedFunc(apps, func(a, b App) int { return int(b.Bytes() - a.Bytes()) }) {
		t.Error("apps aren't sorted largest first")
	}
	i := slices.IndexFunc(apps, func(a App) bool { return a.BundleID == "com.apple.Pages" })
	if i < 0 || !apps[i].Apple() || apps[i].Name != "Pages" {
		t.Errorf("Pages not parsed as an Apple app: %+v", apps)
	}
	if apps[0].Apple() || apps[0].AppBytes == 0 || apps[0].DataBytes == 0 {
		t.Errorf("largest app: %+v", apps[0])
	}
}

func TestParseShared(t *testing.T) {
	shared := Parse(loadFixture(t)).Shared
	if len(shared) != 8 {
		t.Fatalf("got %d containers with files, want 8", len(shared))
	}
	var total int64
	for _, c := range shared {
		total += c.Bytes
		if len(c.Files) == 0 || len(c.Files) > topFiles {
			t.Errorf("%s keeps %d files", c.BundleID, len(c.Files))
		}
		if !slices.IsSortedFunc(c.Files, func(a, b File) int { return int(b.Bytes - a.Bytes) }) {
			t.Errorf("%s files aren't sorted largest first", c.BundleID)
		}
		if c.Name == "" || c.Name == c.BundleID {
			t.Errorf("%s has no app name", c.BundleID)
		}
	}
	if total != 730_792_689 {
		t.Errorf("shared total %d, want 730792689", total)
	}
}

// TestParseLiveTypes uses the value types go-ios returns, which differ from
// the float64s a JSON fixture decodes to.
func TestParseLiveTypes(t *testing.T) {
	p := Parse(Raw{
		Disk: map[string]any{"TotalDiskCapacity": uint64(128_000_000_000), "AmountDataAvailable": uint64(1_000)},
		Apps: []map[string]any{
			{"CFBundleIdentifier": "com.example.a", "CFBundleName": "A", "StaticDiskUsage": uint64(10), "DynamicDiskUsage": uint64(5)},
			{"CFBundleIdentifier": "com.example.b", "StaticDiskUsage": uint64(1)},
		},
		Files: []map[string]any{{"CFBundleIdentifier": "com.example.a", "Name": "x.mov", "Size": int64(7)}},
	})
	if p.TotalBytes != 128_000_000_000 || p.FreeBytes != 1_000 {
		t.Errorf("storage: %+v", p)
	}
	if p.Apps[0].Name != "A" || p.Apps[0].Bytes() != 15 || p.Apps[1].Name != "com.example.b" {
		t.Errorf("apps: %+v", p.Apps)
	}
	if len(p.Shared) != 1 || p.Shared[0].Name != "A" || p.Shared[0].Files[0].Bytes != 7 {
		t.Errorf("shared: %+v", p.Shared)
	}
}
