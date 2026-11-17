// Package engine provides browser-independent transfer operations.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	_, err = Stream(ctx, client, Request{Source: source}, dst, progress, nil)
	return err
}

// Representation describes the identity observed before accepting response bytes.
type Representation struct {
	Effective, ETag, Encoding string
	Length                    int64
}
type Request struct {
	Source   string
	Resume   bool
	Offset   int64
	Identity Representation
	Probe    bool
}

var ErrUnsafeResume = errors.New("source cannot be safely resumed; explicit restart required")

func StrongETag(s string) bool {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return false
	}
	for _, c := range []byte(s[1 : len(s)-1]) {
		if c < 0x21 || c == '"' || c == 0x7f {
			return false
		}
	}
	return true
}

// Stream uses the same bounded HTTP path for initial transfers, probes and resume.
func Stream(ctx context.Context, client *http.Client, request Request, dst io.Writer, progress func(int64, int64), inspect func(Representation) error) (representation Representation, err error) {
	source := request.Source
	u, parseErr := url.Parse(source)
	if parseErr != nil || !validURL(u) {
		return representation, errors.New("invalid HTTP URL")
	}
	if client == nil || dst == nil {
		return representation, errors.New("client and destination are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return representation, &transferError{"create request", err}
	}
	req.Header.Set("Accept-Encoding", "identity")
	if request.Resume {
		if !StrongETag(request.Identity.ETag) {
			return representation, ErrUnsafeResume
		}
		req.Header.Set("If-Range", request.Identity.ETag)
		value := fmt.Sprintf("bytes=%d-", request.Offset)
		if request.Probe {
			value = "bytes=0-0"
		}
		req.Header.Set("Range", value)
	}
	// Copy configuration so redirect policy is enforced without mutating callers.
	bounded := *client
	bounded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// net/http generates Referer before this callback, including signed
		// queries. Downloads never forward it, even to the same origin.
		defer req.Header.Del("Referer")
		if len(via) >= 10 {
			return errors.New("redirect limit reached")
		}
		if !validURL(req.URL) {
			return errors.New("invalid redirect URL")
		}
		if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("insecure redirect")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		return nil
	}
	resp, err := bounded.Do(req)
	if err != nil {
		return representation, &transferError{"request", err}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, &transferError{"close response", closeErr})
		}
	}()
	if !request.Resume && resp.StatusCode != http.StatusOK {
		return representation, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	// Error-page encoding says nothing about the requested representation.
	if request.Resume && resp.StatusCode >= 500 {
		return representation, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	if encoding := resp.Header.Get("Content-Encoding"); len(resp.Header.Values("Content-Encoding")) > 1 || (encoding != "" && encoding != "identity") {
		if request.Resume {
			return representation, ErrUnsafeResume
		}
		return representation, errors.New("unsupported response encoding")
	}
	representation = Representation{ETag: resp.Header.Get("ETag"), Encoding: resp.Header.Get("Content-Encoding"), Length: resp.ContentLength, Effective: source}
	if resp.Request != nil {
		representation.Effective = resp.Request.URL.String()
	}
	if len(resp.Header.Values("ETag")) > 1 {
		representation.ETag = ""
	}
	expected := resp.ContentLength
	if request.Resume {
		start, end, total, ok := contentRange(resp.Header.Get("Content-Range"))
		wantStart := request.Offset
		if request.Probe {
			wantStart = 0
		}
		if resp.StatusCode != 206 || len(resp.Header.Values("Content-Range")) != 1 || !ok || start != wantStart || (!request.Probe && end != total-1) || (request.Probe && end != 0) || representation.ETag != request.Identity.ETag || representation.Effective != request.Identity.Effective || representation.Encoding != request.Identity.Encoding || (request.Identity.Length >= 0 && total != request.Identity.Length) || (resp.ContentLength >= 0 && resp.ContentLength != end-start+1) {
			return representation, ErrUnsafeResume
		}
		representation.Length = total
		expected = end - start + 1
	}
	if inspect != nil {
		if err := inspect(representation); err != nil {
			return representation, err
		}
	}
	buf := make([]byte, 32*1024)
	var written int64
	if progress != nil {
		progress(0, representation.Length)
	}
	for {
		if err := ctx.Err(); err != nil {
			return representation, err
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if expected >= 0 && written+int64(n) > expected {
				return representation, ErrUnsafeResume
			}
			count, writeErr := dst.Write(buf[:n])
			written += int64(count)
			if progress != nil {
				progress(written, representation.Length)
			}
			if writeErr != nil {
				return representation, &transferError{"write", writeErr}
			}
			if count != n {
				return representation, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			if expected >= 0 && written != expected {
				return representation, io.ErrUnexpectedEOF
			}
			return representation, ctx.Err()
		}
		if readErr != nil {
			return representation, &transferError{"read response", readErr}
		}
	}
}

func contentRange(value string) (start, end, total int64, ok bool) {
	if !strings.HasPrefix(value, "bytes ") {
		return
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes "), "/")
	if len(parts) != 2 {
		return
	}
	bounds := strings.Split(parts[0], "-")
	if len(bounds) != 2 {
		return
	}
	for _, part := range []string{bounds[0], bounds[1], parts[1]} {
		for _, c := range part {
			if c < '0' || c > '9' {
				return
			}
		}
	}
	var e error
	start, e = strconv.ParseInt(bounds[0], 10, 64)
	if e != nil {
		return
	}
	end, e = strconv.ParseInt(bounds[1], 10, 64)
	if e != nil {
		return
	}
	total, e = strconv.ParseInt(parts[1], 10, 64)
	if e != nil {
		return
	}
	ok = start >= 0 && end >= start && total > end
	return
}
