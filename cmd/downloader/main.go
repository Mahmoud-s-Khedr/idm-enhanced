// Command downloader downloads one HTTP(S) URL into a new file.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stderr)
	stop()
	os.Exit(code)
}
