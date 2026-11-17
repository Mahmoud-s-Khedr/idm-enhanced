// Package cli presents the sequential downloader without owning transfer policy.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
)

// Run returns a process exit status: 0 success/help, 2 usage, 130 cancellation,
// or 1 transfer/verification/finalization failure. Diagnostics never echo URLs.
func Run(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("downloader", flag.ContinueOnError)
	// flag's default diagnostics echo invalid values, which could be signed URLs.
	flags.SetOutput(io.Discard)
	destination := flags.String("o", "", "destination file (required; must not exist)")
	checksum := flags.String("sha256", "", "expected SHA-256")
	timeout := flags.Duration("timeout", 30*time.Minute, "whole-transfer timeout")
	quiet := flags.Bool("quiet", false, "hide progress")
	usage := func() {
		_, _ = fmt.Fprintln(stderr, "Usage: downloader -o FILE [-sha256 HEX] [-timeout 30m] [-quiet] URL")
	}
	if err := flags.Parse(args); err != nil {
		usage()
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 || *destination == "" || *timeout <= 0 {
		usage()
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	// The standard transport bounds dial/TLS waits and reuses connections. The
	// context bounds response headers and body, including a stalled server.
	client := &http.Client{}
	defer client.CloseIdleConnections()
	var last time.Time
	progress := func(written, total int64) {
		if *quiet || (!last.IsZero() && time.Since(last) < 250*time.Millisecond) {
			return
		}
		last = time.Now()
		if total < 0 {
			_, _ = fmt.Fprintf(stderr, "Progress: %d bytes (total unknown)\n", written)
		} else {
			_, _ = fmt.Fprintf(stderr, "Progress: %d/%d bytes\n", written, total)
		}
	}
	err := engine.Download(ctx, client, engine.DownloadOptions{URL: flags.Arg(0), Destination: *destination, SHA256: *checksum, Progress: progress})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			_, _ = fmt.Fprintln(stderr, "Download cancelled:", err)
			return 130
		case errors.Is(err, context.DeadlineExceeded):
			_, _ = fmt.Fprintln(stderr, "Download timed out:", err)
		default:
			_, _ = fmt.Fprintln(stderr, "Download failed:", err)
		}
		return 1
	}
	_, _ = fmt.Fprintln(stderr, "Download complete.")
	return 0
}
