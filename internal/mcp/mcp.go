// Package mcp serves Groundskeeper's read-only results as MCP tools over
// stdio. It is the only package that imports the MCP SDK.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/report"
)

// Scan runs a read-only scan of the home folder and writes no files.
type Scan func(ctx context.Context) (report.Report, error)

// ErrBusy is what a Scan returns while another scan is running. Its message
// reaches the client as is.
var ErrBusy = errors.New("another scan is already running; try again in a minute")

// Options are what the server needs.
type Options struct {
	Version  string
	Home     string // written as ~ in every result
	StateDir string // where the snapshots are
	Scan     Scan
	Log      *logging.Logger // gets one line per tool call
}

const scanDescription = `Scan this Mac for disk space that can be reclaimed: caches, AI models, ` +
	`stale build artifacts, toolchains, containers, old downloads, browser caches and unused apps. ` +
	`Returns free and total disk space, the reclaimable total, and each item with its size in bytes, ` +
	`a safety tag (safe, review or keep) and a suggested command for the user to run. ` +
	`Takes up to 45 seconds; folders not measured in time are listed as skipped. Read-only.`

// Serve runs the server on in and out, usually stdin and stdout, until the
// client disconnects. out carries only protocol messages.
func Serve(ctx context.Context, opts Options, in io.Reader, out io.Writer) error {
	t := &sdk.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}}
	return NewServer(opts).Run(ctx, t)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// NewServer returns a server with the scan, history and stats tools.
func NewServer(opts Options) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "groundskeeper", Version: opts.Version}, nil)
	s.AddReceivingMiddleware(logCalls(opts.Log))
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}
	sdk.AddTool(s, &sdk.Tool{Name: "scan", Description: scanDescription, Annotations: readOnly},
		recorded(func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, ScanResult, error) {
			rep, err := opts.Scan(ctx)
			if errors.Is(err, ErrBusy) {
				return nil, ScanResult{}, err
			}
			if err != nil {
				return nil, ScanResult{}, fmt.Errorf("scan failed: %s", logging.KindOf(err))
			}
			return nil, resultOf(rep, opts.Home), nil
		}))
	sdk.AddTool(s, &sdk.Tool{Name: "history", Description: historyDescription, Annotations: readOnly},
		recorded(func(_ context.Context, _ *sdk.CallToolRequest, in RunsInput) (*sdk.CallToolResult, HistoryResult, error) {
			return historyOf(opts, in)
		}))
	sdk.AddTool(s, &sdk.Tool{Name: "stats", Description: statsDescription, Annotations: readOnly},
		recorded(func(_ context.Context, _ *sdk.CallToolRequest, in RunsInput) (*sdk.CallToolResult, StatsResult, error) {
			return statsOf(opts, in)
		}))
	return s
}
