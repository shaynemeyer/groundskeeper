package iphone

import (
	"cmp"
	"slices"
	"strings"
)

// topFiles is how many of a shared container's largest files are kept.
const topFiles = 5

// Phone is what one read of an iPhone found.
type Phone struct {
	Model      string      `json:"model"`
	IOS        string      `json:"ios"`
	TotalBytes int64       `json:"total"`
	FreeBytes  int64       `json:"free"`
	Apps       []App       `json:"apps"`   // largest first
	Shared     []Container `json:"shared"` // file-sharing apps holding files, largest first
}

// App is an installed app and the sizes iOS reports for it over USB.
type App struct {
	BundleID  string `json:"bundleID"`
	Name      string `json:"name"`
	AppBytes  int64  `json:"appBytes"`  // the app itself (StaticDiskUsage)
	DataBytes int64  `json:"dataBytes"` // documents and data (DynamicDiskUsage)
}

// Bytes is the app's total size.
func (a App) Bytes() int64 { return a.AppBytes + a.DataBytes }

// Apple reports whether the app is one of Apple's.
func (a App) Apple() bool { return strings.HasPrefix(a.BundleID, "com.apple.") }

// Container is a file-sharing app's Documents folder.
type Container struct {
	BundleID string `json:"bundleID"`
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Count    int    `json:"count"`
	Files    []File `json:"files"` // up to topFiles, largest first
}

// File is a file in a shared container, named relative to Documents.
type File struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Parse turns go-ios responses into a Phone.
func Parse(raw Raw) Phone {
	p := Phone{
		Model:      str(raw.Device["ProductType"]),
		IOS:        str(raw.Device["ProductVersion"]),
		TotalBytes: num(raw.Disk["TotalDiskCapacity"]),
		FreeBytes:  num(raw.Disk["AmountDataAvailable"]),
	}
	names := map[string]string{}
	for _, m := range raw.Apps {
		a := App{
			BundleID:  str(m["CFBundleIdentifier"]),
			Name:      cmp.Or(str(m["CFBundleDisplayName"]), str(m["CFBundleName"]), str(m["CFBundleIdentifier"])),
			AppBytes:  num(m["StaticDiskUsage"]),
			DataBytes: num(m["DynamicDiskUsage"]),
		}
		names[a.BundleID] = a.Name
		p.Apps = append(p.Apps, a)
	}
	slices.SortFunc(p.Apps, func(a, b App) int { return cmp.Compare(b.Bytes(), a.Bytes()) })
	p.Shared = containers(raw.Files, names)
	return p
}

// containers groups shared files by app and keeps each app's largest files.
func containers(files []map[string]any, names map[string]string) []Container {
	byApp := map[string]*Container{}
	for _, m := range files {
		id := str(m["CFBundleIdentifier"])
		c := byApp[id]
		if c == nil {
			c = &Container{BundleID: id, Name: cmp.Or(names[id], id)}
			byApp[id] = c
		}
		f := File{Name: str(m["Name"]), Bytes: num(m["Size"])}
		c.Bytes += f.Bytes
		c.Count++
		c.Files = append(c.Files, f)
	}
	var out []Container
	for _, c := range byApp {
		if c.Bytes == 0 {
			continue
		}
		slices.SortFunc(c.Files, func(a, b File) int { return cmp.Compare(b.Bytes, a.Bytes) })
		c.Files = c.Files[:min(topFiles, len(c.Files))]
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Container) int { return cmp.Compare(b.Bytes, a.Bytes) })
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// num reads a size from go-ios (uint64 or int64) or a JSON fixture (float64).
func num(v any) int64 {
	switch v := v.(type) {
	case uint64:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}
