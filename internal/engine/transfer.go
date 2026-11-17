// Package engine provides browser-independent transfer operations.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// transferError preserves cancellation/error identity without exposing signed URLs
// or request metadata from transport errors in user-facing diagnostics.
type transferError struct {
	operation string
	cause     error
}

func (e *transferError) Error() string { return e.operation + " failed" }
func (e *transferError) Unwrap() error { return e.cause }

func validURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

// Transfer streams one response using a fixed 32 KiB buffer. Negative totals
// mean unknown size. The caller owns dst and its verification/finalization.
// client must be non-nil; its transport should honor request cancellation.
func Transfer(ctx context.Context, client *http.Client, source string, dst io.Writer, progress func(int64, int64)) (err error) {
	u, parseErr := url.Parse(source)
	if parseErr != nil || !validURL(u) {
		return errors.New("invalid HTTP URL")
	}
	if client == nil || dst == nil {
		return errors.New("client and destination are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return &transferError{"create request", err}
	}
	req.Header.Set("Accept-Encoding", "identity")
	// Copy configuration so redirect policy is enforced without mutating callers.
	bounded := *client
	bounded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("redirect limit reached")
		}
		if !validURL(req.URL) {
			return errors.New("invalid redirect URL")
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("insecure redirect")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		return nil
	}
	resp, err := bounded.Do(req)
	if err != nil {
		return &transferError{"request", err}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, &transferError{"close response", closeErr})
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return errors.New("unsupported response encoding")
	}
	buf := make([]byte, 32*1024)
	var written int64
	if progress != nil {
		progress(0, resp.ContentLength)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			count, writeErr := dst.Write(buf[:n])
			written += int64(count)
			if progress != nil {
				progress(written, resp.ContentLength)
			}
			if writeErr != nil {
				return &transferError{"write", writeErr}
			}
			if count != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			if resp.ContentLength >= 0 && written != resp.ContentLength {
				return io.ErrUnexpectedEOF
			}
			return ctx.Err()
		}
		if readErr != nil {
			return &transferError{"read response", readErr}
		}
	}
}
