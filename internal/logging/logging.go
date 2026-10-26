// Package logging writes JSON log lines with log/slog. Fields can only be
// built by the constructors below, so file contents, environment variables
// and raw command output can't be logged by accident.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// DefaultMaxBytes is the size at which the log file is rotated.
const DefaultMaxBytes = 5 << 20

// Field is one safe log attribute.
type Field struct{ attr slog.Attr }

// Path logs a file system path.
func Path(p string) Field { return Field{slog.String("path", p)} }

// Bytes logs a size in bytes.
func Bytes(n int64) Field { return Field{slog.Int64("bytes", n)} }

// Count logs a count under key.
func Count(key string, n int) Field { return Field{slog.Int(key, n)} }

// Duration logs a duration in milliseconds.
func Duration(d time.Duration) Field { return Field{slog.Int64("duration_ms", d.Milliseconds())} }

// ID logs an identifier such as a scanner name or bundle ID under key.
func ID(key, id string) Field { return Field{slog.String(key, id)} }

// Flag logs a boolean under key.
func Flag(key string, v bool) Field { return Field{slog.Bool(key, v)} }

// ErrKind logs the kind of err, never its message.
func ErrKind(err error) Field { return Field{slog.String("error_kind", KindOf(err))} }

// KindOf names the kind of err without its message: "permission",
// "not_exist", "timeout", "canceled" or "other".
func KindOf(err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission"
	case errors.Is(err, fs.ErrNotExist):
		return "not_exist"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "other"
}

// Logger logs messages with safe fields.
type Logger struct{ l *slog.Logger }

// Debug logs at debug level.
func (l *Logger) Debug(msg string, f ...Field) { l.log(slog.LevelDebug, msg, f) }

// Info logs at info level.
func (l *Logger) Info(msg string, f ...Field) { l.log(slog.LevelInfo, msg, f) }

// Warn logs at warn level.
func (l *Logger) Warn(msg string, f ...Field) { l.log(slog.LevelWarn, msg, f) }

// Error logs at error level.
func (l *Logger) Error(msg string, f ...Field) { l.log(slog.LevelError, msg, f) }

func (l *Logger) log(level slog.Level, msg string, fields []Field) {
	attrs := make([]slog.Attr, len(fields))
	for i, f := range fields {
		attrs[i] = f.attr
	}
	l.l.LogAttrs(context.Background(), level, msg, attrs...)
}

// Discard returns a Logger that writes nothing.
func Discard() *Logger { return &Logger{slog.New(slog.DiscardHandler)} }

// Text returns a Logger that writes debug lines as plain text to w only.
func Text(w io.Writer, cmd string) *Logger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	return &Logger{slog.New(h).With("cmd", cmd)}
}

// Options configure Open.
type Options struct {
	Level    string    // "debug", "info", "warn" or "error"
	Cmd      string    // command name added to every line
	Verbose  io.Writer // if set, also gets debug lines as plain text
	MaxBytes int64     // rotation size; 0 means DefaultMaxBytes
}

// Open rotates path if it is too large, then returns a Logger appending to it.
func Open(path string, opts Options) (*Logger, io.Closer, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(opts.Level)); err != nil {
		return nil, nil, fmt.Errorf("logLevel: %w", err)
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	if err := rotate(path, opts.MaxBytes); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return nil, nil, err
	}

	var h slog.Handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	if opts.Verbose != nil {
		text := slog.NewTextHandler(opts.Verbose, &slog.HandlerOptions{Level: slog.LevelDebug})
		h = slog.NewMultiHandler(h, text)
	}
	l := slog.New(h).With("run_id", runID(), "cmd", opts.Cmd)
	return &Logger{l}, f, nil
}

func rotate(path string, maxBytes int64) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || info.Size() < maxBytes {
		return err
	}
	for i := 2; i >= 1; i-- {
		err := os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return os.Rename(path, path+".1")
}

func runID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
