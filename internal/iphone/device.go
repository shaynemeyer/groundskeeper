// Package iphone reads storage, apps and shared files from an iPhone over USB
// with go-ios. It only reads; uninstalling lives in internal/clean.
package iphone

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/afc"
	"github.com/danielpaulus/go-ios/ios/house_arrest"
)

// Errors are reported by kind. go-ios's own messages can hold the device's
// UDID, so they are never shown.
var (
	ErrNoDevice   = errors.New("no iPhone connected over USB")
	ErrNotTrusted = errors.New("iPhone hasn't trusted this Mac")
	ErrRead       = errors.New("couldn't read the iPhone")
)

func init() {
	// go-ios logs the UDID at INFO level through slog.Default unless given
	// its own logger.
	ios.SetLogger(slog.New(slog.DiscardHandler))
}

// Raw holds go-ios responses as they arrive: lockdown values, the
// com.apple.disk_usage domain, user apps with sizes, and the files in each
// file-sharing app's Documents folder. Recordings of it are fixtures.
type Raw struct {
	Device map[string]any   `json:"Device"`
	Disk   map[string]any   `json:"Disk"`
	Apps   []map[string]any `json:"Apps"`
	Files  []map[string]any `json:"Files"`
	// Skipped lists bundle IDs whose shared files couldn't be listed.
	Skipped []string `json:"Skipped,omitempty"`
}

// Read connects to the first iPhone on USB and reads everything Raw holds.
func Read() (Raw, error) {
	device, err := connect()
	if err != nil {
		return Raw{}, err
	}
	var raw Raw
	if raw.Device, err = ios.GetValuesPlist(device); err != nil {
		return Raw{}, readErr("lockdown values", err)
	}
	if raw.Disk, err = diskUsage(device); err != nil {
		return Raw{}, readErr("disk usage", err)
	}
	if raw.Apps, err = browseApps(device); err != nil {
		return Raw{}, readErr("app list", err)
	}
	for _, app := range raw.Apps {
		if shared, _ := app["UIFileSharingEnabled"].(bool); !shared {
			continue
		}
		id, _ := app["CFBundleIdentifier"].(string)
		files, err := sharedFiles(device, id)
		if err != nil {
			raw.Skipped = append(raw.Skipped, id)
			continue
		}
		raw.Files = append(raw.Files, files...)
	}
	return raw, nil
}

// connect finds the first USB device and checks this Mac is paired with it.
func connect() (ios.DeviceEntry, error) {
	list, err := ios.ListDevices()
	if err != nil {
		return ios.DeviceEntry{}, readErr("usbmux", err)
	}
	if len(list.DeviceList) == 0 {
		return ios.DeviceEntry{}, ErrNoDevice
	}
	device := list.DeviceList[0]
	if _, err := ios.ReadPairRecord(device.Properties.SerialNumber); err != nil {
		return ios.DeviceEntry{}, ErrNotTrusted
	}
	return device, nil
}

// readErr wraps ErrRead with the step that failed, dropping go-ios's message.
func readErr(step string, _ error) error {
	return fmt.Errorf("%w: %s failed", ErrRead, step)
}

func diskUsage(device ios.DeviceEntry) (map[string]any, error) {
	conn, err := ios.ConnectLockdownWithSession(device)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	v, err := conn.GetValueForDomain("", "com.apple.disk_usage")
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("disk usage is %T", v)
	}
	return m, nil
}

// browseApps sends its own installation_proxy Browse, because go-ios's
// BrowseUserApps asks for no ReturnAttributes and so gets no sizes.
func browseApps(device ios.DeviceEntry) ([]map[string]any, error) {
	conn, err := ios.ConnectToService(device, "com.apple.mobile.installation_proxy")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	codec := ios.NewPlistCodec()
	req, err := codec.Encode(map[string]any{
		"Command": "Browse",
		"ClientOptions": map[string]any{
			"ApplicationType": "User",
			"ReturnAttributes": []string{
				"CFBundleIdentifier", "CFBundleDisplayName", "CFBundleName",
				"StaticDiskUsage", "DynamicDiskUsage", "UIFileSharingEnabled",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if err := conn.Send(req); err != nil {
		return nil, err
	}
	var apps []map[string]any
	for {
		msg, err := codec.Decode(conn.Reader())
		if err != nil {
			return nil, err
		}
		resp, err := ios.ParsePlist(msg)
		if err != nil {
			return nil, err
		}
		if resp["Error"] != nil {
			return nil, fmt.Errorf("browse error %v", resp["Error"])
		}
		list, _ := resp["CurrentList"].([]any)
		for _, a := range list {
			if m, ok := a.(map[string]any); ok {
				apps = append(apps, m)
			}
		}
		if resp["Status"] == "Complete" {
			return apps, nil
		}
	}
}

// sharedFiles lists the files in an app's Documents folder. house_arrest
// refuses to list "/" for App Store apps, and go-ios's WalkDir hides that
// error, so the walk starts at /Documents.
func sharedFiles(device ios.DeviceEntry, bundleID string) ([]map[string]any, error) {
	c, err := house_arrest.New(device, bundleID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	var files []map[string]any
	err = c.WalkDir("/Documents", func(p string, info afc.FileInfo, _ error) error {
		if info.IsDir() || info.IsLink() {
			return nil
		}
		files = append(files, map[string]any{
			"CFBundleIdentifier": bundleID,
			"Name":               strings.TrimPrefix(p, "/Documents/"),
			"Size":               info.Size,
		})
		return nil
	})
	return files, err
}
