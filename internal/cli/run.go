// Package cli presents download commands without owning transfer policy.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
)

// Run returns a process exit status: 0 success/help, 2 usage, 130 cancellation,
// or 1 transfer/verification/finalization failure. Diagnostics never echo URLs.
func Run(ctx context.Context, args []string, stderr io.Writer) (code int) {
	stateDir := ""
	if len(args) > 0 && (args[0] == "--state-dir" || args[0] == "-state-dir") {
		if len(args) < 2 {
			return 2
		}
		stateDir = args[1]
		args = args[2:]
	}
	if stateDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return 1
		}
		stateDir = filepath.Join(dir, "idm-enhanced")
	}
	command := "download"
	if len(args) > 0 {
		switch args[0] {
		case "resume", "restart", "status", "cancel", "enqueue", "run-queue", "reorder":
			command = args[0]
			args = args[1:]
		}
	}
	flags := flag.NewFlagSet("downloader", flag.ContinueOnError)
	// flag's default diagnostics echo invalid values, which could be signed URLs.
	flags.SetOutput(io.Discard)
	destination := flags.String("o", "", "destination file (required; must not exist)")
	checksum := flags.String("sha256", "", "expected SHA-256")
	timeout := flags.Duration("timeout", 30*time.Minute, "whole-transfer timeout")
	keep := flags.Bool("keep-partial", false, "retain partial")
	discard := flags.Bool("discard-partial", false, "discard partial")
	config := manager.DefaultConfig()
	workers := flags.Int("workers", config.Workers, "range workers (1-16)")
	jobsLimit := flags.Int("jobs", config.Jobs, "simultaneous jobs (1-16)")
	perHost := flags.Int("per-host", config.PerHost, "simultaneous requests per host")
	rate := flags.Int64("rate", 0, "global body bytes per second (0 unlimited)")
	retries := flags.Int("retries", config.Retries, "automatic retry count (0-8)")
	quiet := flags.Bool("quiet", false, "hide progress")
	usage := func() {
		_, _ = fmt.Fprintln(stderr, "Usage: downloader [--state-dir DIR] -o FILE [-sha256 HEX] [-timeout 30m] [-quiet] URL; enqueue [flags] URL; resume|restart|status|cancel [flags] JOB_ID; run-queue [flags]; reorder JOB_ID...")
	}
	if err := flags.Parse(args); err != nil {
		usage()
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *timeout <= 0 || ((command == "download" || command == "enqueue") && (flags.NArg() != 1 || *destination == "")) || ((command == "resume" || command == "restart" || command == "cancel") && flags.NArg() != 1) || (command == "status" && flags.NArg() > 1) || (command == "run-queue" && flags.NArg() != 0) || (command == "cancel" && *keep == *discard) {
		usage()
		return 2
	}
	config.Workers = *workers
	config.Jobs = *jobsLimit
	config.PerHost = *perHost
	config.BytesPerSecond = *rate
	config.Retries = *retries

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
	m, err := manager.OpenContext(ctx, stateDir, command == "status", client)
	if err != nil {
		return reportError(stderr, err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			_, _ = fmt.Fprintln(stderr, "State close failed")
			if code == 0 {
				code = 1
			}
		}
	}()
	if err := m.Configure(config, client); err != nil {
		return reportError(stderr, err)
	}
	switch command {
	case "run-queue":
		err = m.RunQueue(ctx)
	case "reorder":
		err = m.Reorder(ctx, flags.Args())
	case "status":
		var jobs []manager.Job
		jobs, err = m.Status(ctx)
		found := flags.NArg() == 0
		for _, j := range jobs {
			if flags.NArg() == 0 || j.ID == flags.Arg(0) {
				found = true
				_, _ = fmt.Fprintf(stderr, "%s state=%s durable=%d destination=%q resumable=%t error=%s order=%d retries=%d retry_at=%s\n", j.ID, j.State, j.Durable, j.Destination, manager.Resumable(j), manager.SafeError(j), j.QueueOrder, j.RetryCount, j.RetryAt.Format(time.RFC3339))
			}
		}
		if err == nil && !found {
			err = errors.New("job not found")
		}
	case "cancel":
		err = m.Cancel(ctx, flags.Arg(0), *keep)
	case "resume", "restart":
		err = m.Run(ctx, flags.Arg(0), command == "restart", progress)
	default:
		var j manager.Job
		j, err = m.Create(ctx, manager.Options{URL: flags.Arg(0), Destination: *destination, SHA256: *checksum})
		if err == nil {
			_, _ = fmt.Fprintln(stderr, "Job:", j.ID)
			if command != "enqueue" {
				err = m.Run(ctx, j.ID, false, progress)
			}
		}
	}
	if err != nil {
		return reportError(stderr, err)
	}
	if command == "download" || command == "resume" || command == "restart" {
		_, _ = fmt.Fprintln(stderr, "Download complete.")
	}
	return 0
}

func reportError(stderr io.Writer, err error) int {
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
