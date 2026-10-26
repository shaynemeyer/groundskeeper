// Command groundskeeper keeps a Mac's disk free of clutter.
package main

import (
	"context"
	"os"

	"charm.land/fang/v2"

	"github.com/shaynemeyer/groundskeeper/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	err := fang.Execute(context.Background(), cli.NewRoot(),
		fang.WithVersion(version),
		fang.WithNotifySignal(os.Interrupt),
	)
	if err != nil {
		os.Exit(1)
	}
}
