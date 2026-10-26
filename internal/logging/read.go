package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
)

const followEvery = 500 * time.Millisecond

// Entry is one parsed log line.
type Entry struct {
	Time  time.Time
	Level slog.Level
	Msg   string
	RunID string
	Cmd   string
	Attrs []string // other fields as key=value, sorted
	Raw   string
}

// Filter selects entries. The zero MinLevel is info, as in slog; empty
// strings and a zero Limit don't filter.
type Filter struct {
	MinLevel slog.Level
	Run      string // a run ID, or "last" for the newest run
	Cmd      string // command prefix, such as "clean"
	Limit    int    // keep only the newest Limit entries
}

// Read returns the entries in path that match f, oldest first, and the
// offset reading stopped at, for Follow.
func Read(path string, f Filter) ([]Entry, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	all, offset := parseFrom(file, 0)
	if f.Run == "last" {
		f.Run = ""
		if len(all) > 0 {
			f.Run = all[len(all)-1].RunID
		}
	}
	var out []Entry
	for _, e := range all {
		if f.match(e) {
			out = append(out, e)
		}
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out, offset, nil
}

// Follow calls fn for each new matching entry written after offset, until
// ctx is done. A file that shrinks was rotated and is read from the start.
func Follow(ctx context.Context, path string, offset int64, f Filter, fn func(Entry)) error {
	for {
		if info, err := os.Stat(path); err == nil {
			if info.Size() < offset {
				offset = 0
			}
			if info.Size() > offset {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				var entries []Entry
				entries, offset = parseFrom(file, offset)
				file.Close()
				for _, e := range entries {
					if f.match(e) {
						fn(e)
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(followEvery):
		}
	}
}

// parseFrom reads complete lines after offset. A line still being written
// is left for the next read.
func parseFrom(file *os.File, offset int64) ([]Entry, int64) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	r := bufio.NewReader(file)
	var entries []Entry
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return entries, offset
		}
		offset += int64(len(line))
		if e, ok := parse(strings.TrimSuffix(line, "\n")); ok {
			entries = append(entries, e)
		}
	}
}

func parse(line string) (Entry, bool) {
	// UseNumber keeps sizes like 43041574912 from printing as 4.3e+10.
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var m map[string]any
	if dec.Decode(&m) != nil {
		return Entry{}, false
	}
	e := Entry{Raw: line}
	e.Time, _ = time.Parse(time.RFC3339Nano, str(m["time"]))
	e.Level.UnmarshalText([]byte(str(m["level"])))
	e.Msg, e.RunID, e.Cmd = str(m["msg"]), str(m["run_id"]), str(m["cmd"])
	for k, v := range m {
		if !slices.Contains([]string{"time", "level", "msg", "run_id", "cmd"}, k) {
			e.Attrs = append(e.Attrs, fmt.Sprintf("%s=%v", k, v))
		}
	}
	slices.Sort(e.Attrs)
	return e, true
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (f Filter) match(e Entry) bool {
	return e.Level >= f.MinLevel &&
		(f.Run == "" || e.RunID == f.Run) &&
		(f.Cmd == "" || strings.HasPrefix(e.Cmd, f.Cmd))
}
