package scan

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// lastActivity returns when a project was last worked on and how that was
// measured: the last git commit, or else the newest file outside artifact
// folders. Folder mtimes aren't used because a sync or pull resets them.
func lastActivity(ctx context.Context, project string) (time.Time, string) {
	if _, err := os.Stat(filepath.Join(project, ".git")); err == nil {
		if t, err := lastCommit(ctx, project); err == nil {
			return t, "last commit"
		}
	}
	return newestFile(project), "last file change"
}

// lastCommit asks git for the newest commit time. fsmonitor is turned off so
// a repo's own config can't make git run a program.
func lastCommit(ctx context.Context, project string) (time.Time, error) {
	out, err := exec.CommandContext(ctx, "git", "-c", "core.fsmonitor=false",
		"-C", project, "log", "-1", "--format=%ct").Output()
	if err != nil {
		return time.Time{}, err
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0), nil
}

func newestFile(project string) time.Time {
	var newest time.Time
	filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != project && (d.Name() == ".git" || artifactKind(path, d.Name()) != "") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}
