// Command mcpspikeclient drives mcpspike for the step 16 spike. Delete after the spike.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(os.Args[1])}, nil)
	if err != nil {
		panic(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		panic(err)
	}
	for _, t := range tools.Tools {
		fmt.Printf("tool %s readOnly=%v\n", t.Name, t.Annotations.ReadOnlyHint)
	}
	for _, call := range []mcp.CallToolParams{
		{Name: "wait", Arguments: map[string]any{"seconds": 1}},
		{Name: "scan", Arguments: map[string]any{"top": 5}},
		{Name: "scan", Arguments: map[string]any{}},
	} {
		start := time.Now()
		res, err := session.CallTool(ctx, &call)
		if err != nil {
			panic(err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		home, _ := os.UserHomeDir()
		fmt.Printf("%s %v: %s, %d bytes, isError=%v, holds home=%v\n", call.Name, call.Arguments,
			time.Since(start).Round(time.Millisecond), len(text), res.IsError, strings.Contains(text, home))
	}
}
