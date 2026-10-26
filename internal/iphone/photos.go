package iphone

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"time"
)

// Photo sources, as saved in photosSource.
const (
	SourceDevice     = "device"
	SourceMacLibrary = "mac-library"
)

// Errors photos-helper reports by kind.
var (
	ErrLocked  = errors.New("iPhone is locked")
	ErrDenied  = errors.New("the terminal app doesn't have Photos access")
	ErrTimeout = errors.New("the iPhone didn't send its photos in time")
	ErrHelper  = errors.New("photos-helper failed")
)

// Asset is one photo or video from photos-helper list. Favorite, Edited and
// InAlbum are only known for the Mac library.
type Asset struct {
	ID           string    `json:"id"`
	Created      time.Time `json:"created"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Bytes        int64     `json:"bytes"`
	SidecarBytes int64     `json:"sidecarBytes"`
	Burst        string    `json:"burst"`
	Favorite     bool      `json:"favorite"`
	Edited       bool      `json:"edited"`
	InAlbum      bool      `json:"inAlbum"`
}

// Analysis is Vision's result for one asset.
type Analysis struct {
	ID           string  `json:"id"`
	FeaturePrint []byte  `json:"featurePrint"` // 768 little-endian float32 values
	Aesthetics   float64 `json:"aesthetics"`   // -1 to 1
	Utility      bool    `json:"utility"`
}

// Failure is an asset photos-helper couldn't handle, with the step that failed.
type Failure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// HelperRunner runs photos-helper with stdin; tests replace it.
type HelperRunner func(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error)

// Helper runs photos-helper for one source.
type Helper struct {
	Path   string
	Source string
	Run    HelperRunner
}

// RunHelper runs a command without a shell and returns its stdout, which
// holds photos-helper's JSON even when it exits 1.
func RunHelper(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	return cmd.Output()
}

// List returns every photo and video in the source.
func (h Helper) List(ctx context.Context) ([]Asset, error) {
	var out struct{ Assets []Asset }
	err := h.call(ctx, nil, &out, "list")
	return out.Assets, err
}

// Analyze returns Vision results for ids, and the ones that failed.
func (h Helper) Analyze(ctx context.Context, ids []string) ([]Analysis, []Failure, error) {
	in, err := json.Marshal(ids)
	if err != nil {
		return nil, nil, err
	}
	var out struct {
		Analyses []Analysis
		Failed   []Failure
	}
	err = h.call(ctx, in, &out, "analyze")
	return out.Analyses, out.Failed, err
}

// call runs one command and decodes its answer into out.
func (h Helper) call(ctx context.Context, stdin []byte, out any, command string) error {
	data, runErr := h.Run(ctx, stdin, h.Path, command, "--source", h.Source)
	var answer struct{ Error string }
	if json.Unmarshal(data, &answer) == nil && answer.Error != "" {
		return helperError(answer.Error)
	}
	if runErr != nil {
		return fmt.Errorf("%w: %s didn't finish", ErrHelper, command)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: %s gave an unreadable answer", ErrHelper, command)
	}
	return nil
}

func helperError(kind string) error {
	switch kind {
	case "no-device":
		return ErrNoDevice
	case "locked":
		return ErrLocked
	case "denied":
		return ErrDenied
	case "timeout":
		return ErrTimeout
	}
	return fmt.Errorf("%w: unknown error", ErrHelper)
}

// Distance is Vision's feature-print distance: the squared Euclidean
// distance of the two float32 vectors. It is +Inf when their lengths differ.
func Distance(a, b []byte) float64 {
	if len(a) != len(b) || len(a)%4 != 0 {
		return math.Inf(1)
	}
	var sum float64
	for i := 0; i < len(a); i += 4 {
		x := math.Float32frombits(binary.LittleEndian.Uint32(a[i:]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
		d := float64(x - y)
		sum += d * d
	}
	return sum
}
