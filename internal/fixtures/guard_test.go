package fixtures

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFindsPersonalData(t *testing.T) {
	cases := map[string]string{
		"udid40":       `{"UniqueDeviceID": "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}`,
		"udid new":     `udid=00008030-001A2C3E0E12802E`,
		"mac":          `"WiFiAddress": "a4:83:e7:12:34:56"`,
		"email":        `"AppleID": "someone@icloud.com"`,
		"bundle id":    `"CFBundleIdentifier": "com.spotify.client"`,
		"phone json":   `"PhoneNumber": "+1 (415) 555-2671"`,
		"imei json":    `"InternationalMobileEquipmentIdentity": "356938035643809"`,
		"meid plist":   "<key>MobileEquipmentIdentifier</key>\n\t<string>35693803564380</string>",
		"phone number": `"PhoneNumber": 4155552671`,
	}
	for name, in := range cases {
		if got := Check([]byte(in)); len(got) == 0 {
			t.Errorf("%s: Check(%q) found nothing", name, in)
		}
	}
}

func TestCheckAllowsSafeData(t *testing.T) {
	cases := map[string]string{
		"apple bundle":   `"CFBundleIdentifier": "com.apple.mobilesafari"`,
		"example bundle": `"CFBundleIdentifier": "com.example.app1"`,
		"large size":     `{"DynamicDiskUsage": 1234567890123, "StaticDiskUsage": 356938035643809}`,
		"fake phone":     `"PhoneNumber": "` + FakePhone + `"`,
		"fake imei":      `"InternationalMobileEquipmentIdentity": "` + FakeIMEI + `"`,
		"fake meid":      "<key>MobileEquipmentIdentifier</key><string>" + FakeMEID + "</string>",
		"sha256":         `"sha": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"`,
		"go module":      `github.com/danielpaulus/go-ios`,
	}
	for name, in := range cases {
		if got := Check([]byte(in)); len(got) != 0 {
			t.Errorf("%s: Check(%q) = %v, want nothing", name, in, got)
		}
	}
}

// TestTestdataHasNoPersonalData walks every testdata directory in the repo,
// so an unscrubbed recording fails even if the recorder was bypassed.
func TestTestdataHasNoPersonalData(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "bin") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.Contains(filepath.ToSlash(path), "/testdata/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, f := range Check(data) {
			t.Errorf("%s: %s", path, f)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
