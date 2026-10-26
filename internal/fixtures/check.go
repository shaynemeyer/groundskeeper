// Package fixtures keeps personal and device data out of testdata. Check
// finds values that look like a real device's or person's, and the guard
// test runs it over every testdata directory.
package fixtures

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Fake values that recordings use in place of real ones. The phone number is
// in the 555-0100 to 555-0199 range reserved for fiction.
const (
	FakePhone = "+15555550100"
	FakeIMEI  = "000000000000000"
	FakeMEID  = "00000000000000"
)

// Values with a distinctive shape are matched anywhere.
var (
	udid40   = regexp.MustCompile(`\b[0-9a-fA-F]{40}\b`)
	udidNew  = regexp.MustCompile(`\b0000[0-9]{4}-[0-9a-fA-F]{16}\b`)
	mac      = regexp.MustCompile(`\b(?:[0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}\b`)
	email    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	bundleID = regexp.MustCompile(`(?:^|[^A-Za-z0-9.-])((?:com|net|org|io|co|me|app|dev|tv|us|uk|de|fr|jp)\.[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+)`)
)

// Phone numbers and IMEIs look like sizes, so they are only checked in
// fields whose key names them.
const keyedName = `[A-Za-z0-9]*(?:PhoneNumber|IMEI|MEID|MobileEquipmentIdentity|MobileEquipmentIdentifier)[A-Za-z0-9]*`

var (
	keyedJSON  = regexp.MustCompile(`(?i)"(` + keyedName + `)"\s*:\s*(?:"([^"]*)"|([0-9]+))`)
	keyedPlist = regexp.MustCompile(`(?i)<key>(` + keyedName + `)</key>\s*<(?:string|integer)>([^<]*)</`)
)

var allowedBundlePrefixes = []string{"com.apple.", "com.example."}

var fakes = []string{"", FakePhone, FakeIMEI, FakeMEID}

// Check returns a description of each value in data that looks like real
// device or personal data.
func Check(data []byte) []string {
	s := string(data)
	var found []string
	for name, re := range map[string]*regexp.Regexp{"UDID": udid40, "new-format UDID": udidNew, "MAC address": mac, "email": email} {
		for _, m := range re.FindAllString(s, -1) {
			found = append(found, fmt.Sprintf("%s %q", name, m))
		}
	}
	for _, m := range bundleID.FindAllStringSubmatch(s, -1) {
		if !hasAnyPrefix(m[1], allowedBundlePrefixes) {
			found = append(found, fmt.Sprintf("bundle ID %q", m[1]))
		}
	}
	for _, re := range []*regexp.Regexp{keyedJSON, keyedPlist} {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			value := strings.Join(m[2:], "")
			if !slices.Contains(fakes, value) {
				found = append(found, fmt.Sprintf("%s %q is not the fixed fake", m[1], value))
			}
		}
	}
	return found
}

func hasAnyPrefix(s string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(s, p) })
}
