package fixtures

import (
	"encoding/json"
	"strings"
	"testing"
)

// sample mixes lockdown values, an app list and a shared-file list the way
// go-ios returns them, with every kind of personal data the plan names.
func sample() map[string]any {
	return map[string]any{
		"Device": map[string]any{
			"UniqueDeviceID":                       "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
			"SerialNumber":                         "F2LXK0QJHG7F",
			"UniqueChipID":                         uint64(6198473021938745),
			"InternationalMobileEquipmentIdentity": "356938035643809",
			"PhoneNumber":                          "+1 (415) 555-2671",
			"WiFiAddress":                          "a4:83:e7:12:34:56",
			"BluetoothAddress":                     "a4:83:e7:12:34:57",
			"DeviceName":                           "Alex's iPhone",
			"ProductVersion":                       "27.0.1",
			"ProductType":                          "iPhone17,1",
			"TunnelUDID":                           "00008030-001A2C3E0E12802E",
		},
		"Disk": map[string]any{"FSTotalBytes": uint64(256_000_000_000), "FSFreeBytes": uint64(41_234_567_890)},
		"Apps": []any{
			map[string]any{
				"CFBundleIdentifier":   "com.spotify.client",
				"CFBundleDisplayName":  "Spotify",
				"StaticDiskUsage":      uint64(180_000_000),
				"DynamicDiskUsage":     uint64(9_876_543_210),
				"Path":                 "/private/var/containers/Bundle/Application/1A2B/Spotify.app",
				"EnvironmentVariables": map[string]any{"HOME": "/private/var/mobile/Containers/Data/1A2B"},
				"SignerIdentity":       "Apple iPhone OS Application Signing",
				"AppleID":              "alex@icloud.com",
			},
			map[string]any{
				"CFBundleIdentifier":  "com.apple.mobilesafari",
				"CFBundleDisplayName": "Safari",
				"StaticDiskUsage":     uint64(1_000),
			},
		},
		"Files": []any{
			map[string]any{"CFBundleIdentifier": "com.spotify.client", "Name": "Alex wedding.mov", "Size": uint64(2_000_000_000)},
		},
	}
}

func scrubbed(t *testing.T) (string, map[string]any) {
	t.Helper()
	out, err := New().Scrub(sample())
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return string(b), m
}

func TestScrubRemovesPersonalData(t *testing.T) {
	got, _ := scrubbed(t)
	originals := []string{
		"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", "F2LXK0QJHG7F", "6198473021938745",
		"356938035643809", "00008030-001A2C3E0E12802E", "415", "a4:83:e7", "Alex", "alex@icloud.com",
		"com.spotify.client", "Spotify", "1A2B", "wedding", "Signing",
	}
	for _, s := range originals {
		if strings.Contains(got, s) {
			t.Errorf("scrubbed output still contains %q:\n%s", s, got)
		}
	}
	if f := Check([]byte(got)); len(f) != 0 {
		t.Errorf("Check flags scrubbed output: %v", f)
	}
}

func TestScrubKeepsSizesAndAppleApps(t *testing.T) {
	got, m := scrubbed(t)
	for _, s := range []string{"256000000000", "41234567890", "180000000", "9876543210", "2000000000", "com.apple.mobilesafari", "Safari", "27.0.1", "iPhone17,1"} {
		if !strings.Contains(got, s) {
			t.Errorf("scrubbed output lost %q:\n%s", s, got)
		}
	}
	app := m["Apps"].([]any)[0].(map[string]any)["CFBundleIdentifier"]
	file := m["Files"].([]any)[0].(map[string]any)["CFBundleIdentifier"]
	if app != file || !strings.HasPrefix(app.(string), "com.example.") {
		t.Errorf("third-party bundle ID mapped to %v in apps and %v in files, want the same com.example ID", app, file)
	}
	if name := m["Files"].([]any)[0].(map[string]any)["Name"]; !strings.HasSuffix(name.(string), ".mov") {
		t.Errorf("file name scrubbed to %v, want the extension kept", name)
	}
}

func TestScrubRefusesWhatCheckFlags(t *testing.T) {
	in := map[string]any{"ProductVersion": "contact alex@icloud.com"}
	if _, err := New().Scrub(in); err == nil {
		t.Error("Scrub kept an email in an allowed field without an error")
	}
}
