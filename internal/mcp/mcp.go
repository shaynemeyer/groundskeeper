// Package mcp serves Groundskeeper's read-only results as MCP tools over
// stdio. It is the only package that imports the MCP SDK.
package mcp

import (
	"context"
	"fmt"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
	"github.com/shaynemeyer/groundskeeper/internal/report"
)

// Scan runs a read-only scan of the home folder and writes no files.
type Scan func(ctx context.Context) (report.Report, error)

const scanDescription = `Scan this Mac for disk space that can be reclaimed: caches, AI models, ` +
	`stale build artifacts, toolchains, containers, old downloads, browser caches and unused apps. ` +
	`Returns free and total disk space, the reclaimable total, and each item with its size in bytes, ` +
	`a safety tag (safe, review or keep) and a suggested command for the user to run. ` +
	`Takes up to 45 seconds; folders not measured in time are listed as skipped. Read-only.`

// Serve runs the server on in and out, usually stdin and stdout, until the
// client disconnects. out carries only protocol messages.
func Serve(ctx context.Context, version, home string, scan Scan, in io.Reader, out io.Writer) error {
	t := &sdk.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}}
	return NewServer(version, home, scan).Run(ctx, t)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// NewServer returns a server with the scan tool. home is written as ~ in
// every result.
func NewServer(version, home string, scan Scan) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "groundskeeper", Version: version}, nil)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "scan",
		Description: scanDescription,
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, ScanResult, error) {
		rep, err := scan(ctx)
		if err != nil {
			return nil, ScanResult{}, fmt.Errorf("scan failed: %s", logging.KindOf(err))
		}
		return nil, resultOf(rep, home), nil
	})
	return s
}
