package fixtures

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Redacted replaces identifiers that have no fixed fake.
const Redacted = "redacted"

// fakeKeys are identifiers replaced with a fixed fake.
var fakeKeys = map[string]string{
	"UniqueDeviceID":                        Redacted,
	"SerialNumber":                          Redacted,
	"MLBSerialNumber":                       Redacted,
	"UniqueChipID":                          Redacted,
	"ChipSerialNo":                          Redacted,
	"DeviceName":                            "iPhone",
	"UserAssignedDeviceName":                "iPhone",
	"WiFiAddress":                           Redacted,
	"BluetoothAddress":                      Redacted,
	"EthernetAddress":                       Redacted,
	"PhoneNumber":                           FakePhone,
	"InternationalMobileEquipmentIdentity":  FakeIMEI,
	"InternationalMobileEquipmentIdentity2": FakeIMEI,
	"MobileEquipmentIdentifier":             FakeMEID,
	"InternationalMobileSubscriberIdentity": Redacted,
	"IntegratedCircuitCardIdentity":         Redacted,
}

var bundleKeys = []string{"CFBundleIdentifier", "ApplicationIdentifier"}

// nameKeys hold an app's name, which is kept only for Apple's apps.
var nameKeys = []string{"CFBundleDisplayName", "CFBundleName", "CFBundleExecutable"}

// keepKeys are copied as they are, after scrubbing anything inside them.
// Every other key is dropped, so fields added by a new iOS never leak.
var keepKeys = []string{
	"Device", "Disk", "Apps", "Files",
	"ProductVersion", "ProductType", "BuildVersion", "DeviceClass",
	"FSTotalBytes", "FSFreeBytes", "FSBlockSize", "Model",
	"TotalDiskCapacity", "TotalDataCapacity", "TotalDataAvailable", "AmountDataAvailable", "AmountDataReserved",
	"ApplicationType", "StaticDiskUsage", "DynamicDiskUsage", "UIFileSharingEnabled",
	"CFBundleShortVersionString", "CFBundleVersion",
	"Size",
}

// Scrubber replaces personal data in go-ios responses before they are saved
// as fixtures. One Scrubber maps each third-party bundle ID to the same
// com.example ID across everything it scrubs.
type Scrubber struct {
	bundles map[string]string
	files   int
}

// New returns a Scrubber with no mappings yet.
func New() *Scrubber {
	return &Scrubber{bundles: map[string]string{}}
}

// Scrub returns a copy of v with only allowed keys, identifiers replaced, and
// third-party bundle IDs, app names and file names made up. It fails if the
// result still contains anything Check flags.
func (s *Scrubber) Scrub(v any) (any, error) {
	out := s.value(v)
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if found := Check(b); len(found) > 0 {
		return nil, fmt.Errorf("scrubbed data still contains %s", strings.Join(found, ", "))
	}
	return out, nil
}

func (s *Scrubber) value(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return s.dict(v)
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = s.value(e)
		}
		return out
	default:
		return v
	}
}

func (s *Scrubber) dict(m map[string]any) map[string]any {
	bundle, _ := m["CFBundleIdentifier"].(string)
	out := map[string]any{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := m[k]
		switch {
		case fakeKeys[k] != "":
			out[k] = fakeKeys[k]
		case slices.Contains(bundleKeys, k):
			out[k] = s.bundle(fmt.Sprint(v))
		case slices.Contains(nameKeys, k):
			out[k] = s.appName(bundle, fmt.Sprint(v))
		case k == "Name":
			out[k] = s.fileName(fmt.Sprint(v))
		case slices.Contains(keepKeys, k):
			out[k] = s.value(v)
		}
	}
	return out
}

func (s *Scrubber) bundle(id string) string {
	if isApple(id) {
		return id
	}
	if fake, ok := s.bundles[id]; ok {
		return fake
	}
	fake := fmt.Sprintf("com.example.app%d", len(s.bundles)+1)
	s.bundles[id] = fake
	return fake
}

func (s *Scrubber) appName(bundle, name string) string {
	if isApple(bundle) {
		return name
	}
	return "App " + strings.TrimPrefix(s.bundle(bundle), "com.example.app")
}

func (s *Scrubber) fileName(name string) string {
	s.files++
	return fmt.Sprintf("file%d%s", s.files, filepath.Ext(name))
}

func isApple(id string) bool {
	return strings.HasPrefix(id, "com.apple.")
}
