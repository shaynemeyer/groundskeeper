package mcp

import (
	"context"
	"errors"
	"slices"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shaynemeyer/groundskeeper/internal/logging"
)

var toolNames = []string{"scan", "history", "stats"}

type outcomeKey struct{}

// logCalls is receiving middleware that logs one line per tool call: the
// tool, how long it took and the outcome. The tool's handler records the
// outcome (see recorded); a call it never saw, because the SDK refused its
// arguments or the tool doesn't exist, is "invalid". Arguments and error
// messages aren't logged.
func logCalls(log *logging.Logger) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			call, ok := req.(*sdk.CallToolRequest)
			if !ok {
				return next(ctx, method, req)
			}
			start := time.Now()
			outcome := new(string)
			res, err := next(context.WithValue(ctx, outcomeKey{}, outcome), method, req)
			if *outcome == "" {
				*outcome = "invalid"
			}
			tool := "unknown"
			if slices.Contains(toolNames, call.Params.Name) {
				tool = call.Params.Name
			}
			fields := []logging.Field{logging.ID("tool", tool), logging.Duration(time.Since(start)), logging.ID("outcome", *outcome)}
			if *outcome == "ok" {
				log.Info("mcp tool call", fields...)
			} else {
				log.Warn("mcp tool call", fields...)
			}
			return res, err
		}
	}
}

// recorded wraps a tool's handler to record its outcome for logCalls: ok,
// busy, rejected or failed.
func recorded[In, Out any](h sdk.ToolHandlerFor[In, Out]) sdk.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		res, out, err := h(ctx, req, in)
		outcome := "ok"
		switch {
		case errors.Is(err, ErrBusy):
			outcome = "busy"
		case errors.Is(err, errRuns):
			outcome = "rejected"
		case err != nil:
			outcome = "failed"
		}
		*ctx.Value(outcomeKey{}).(*string) = outcome
		return res, out, err
	}
}
