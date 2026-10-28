package mcp

import (
	"strings"

	"github.com/shaynemeyer/groundskeeper/internal/report"
	"github.com/shaynemeyer/groundskeeper/internal/scan"
)

// ScanResult is what the scan tool returns: the report's safe values, with
// the home folder written as ~.
type ScanResult struct {
	DiskFree    int64  `json:"diskFreeBytes"`
	DiskTotal   int64  `json:"diskTotalBytes"`
	Reclaimable int64  `json:"reclaimableBytes"`
	Items       []Item `json:"items"`
	Skipped     []Skip `json:"skipped"`
	Advice      string `json:"advice,omitempty"`
}

// Item is one row of the report.
type Item struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Safety  string `json:"safety"`
	Note    string `json:"note,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// Skip is a folder the scan couldn't measure, and why.
type Skip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

const permissionAdvice = "Some folders were skipped for permission. Grant Full Disk Access to ~/bin/groundskeeper " +
	"in System Settings > Privacy & Security; the client app's own access doesn't apply."

func resultOf(rep report.Report, home string) ScanResult {
	res := ScanResult{
		DiskFree:    rep.Disk.Free,
		DiskTotal:   rep.Disk.Total,
		Reclaimable: rep.Reclaimable(),
		Items:       []Item{},
		Skipped:     []Skip{},
	}
	for _, s := range rep.Sections {
		for _, it := range s.Items {
			res.Items = append(res.Items, Item{
				Section: it.Section,
				Label:   it.Label,
				Path:    tilde(it.Path, home),
				Bytes:   it.Bytes,
				Safety:  it.Safety,
				Note:    it.Note,
				Hint:    tildeHint(it.Hint, home),
			})
		}
	}
	for _, sk := range rep.Skipped {
		res.Skipped = append(res.Skipped, Skip{Path: tilde(sk.Path, home), Reason: sk.Reason})
		if sk.Reason == "permission" {
			res.Advice = permissionAdvice
		}
	}
	return res
}

// tilde writes a path under home as ~/rest.
func tilde(path, home string) string {
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}

// tildeHint rewrites quoted paths under home as ~/'rest', which the shell
// still expands; '~/rest' would not be. Hints quote every path with
// scan.ShellQuote, so the quoted home folder is what to look for.
func tildeHint(hint, home string) string {
	quoted := scan.ShellQuote(home)
	hint = strings.ReplaceAll(hint, quoted, "~")
	open := strings.TrimSuffix(scan.ShellQuote(home+"/"), "'")
	return strings.ReplaceAll(hint, open, "~/'")
}
