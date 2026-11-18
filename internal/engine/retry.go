package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type HTTPError struct {
	Status int
	After  time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("server returned HTTP %d", e.Status) }
func statusError(r *http.Response) error {
	var delay time.Duration
	value := r.Header.Get("Retry-After")
	if n, err := strconv.ParseInt(value, 10, 32); err == nil && n > 0 {
		delay = time.Duration(n) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		delay = max(0, time.Until(date))
	}
	return &HTTPError{r.StatusCode, delay}
}

// RetryDelay classifies remote failures only. Disk, protocol, authentication and
// caller cancellation errors never authorize an automatic retry.
func RetryDelay(err error) (time.Duration, bool) {
	if errors.Is(err, ErrUnsafeResume) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 0, false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var delay time.Duration
		for _, cause := range joined.Unwrap() {
			d, retry := RetryDelay(cause)
			if !retry {
				return 0, false
			}
			delay = max(delay, d)
		}
		return delay, len(joined.Unwrap()) > 0
	}
	var status *HTTPError
	if errors.As(err, &status) {
		return status.After, status.Status == 408 || status.Status == 429 || status.Status == 500 || status.Status == 502 || status.Status == 503 || status.Status == 504
	}
	var transfer *transferError
	if errors.As(err, &transfer) && transfer.operation != "request" && transfer.operation != "read response" {
		return 0, false
	}
	// url.Error implements net.Error even for permanent TLS/URL failures. Classify
	// its underlying cause, rather than retrying every failed HTTP request.
	var request *url.Error
	if errors.As(err, &request) {
		return RetryDelay(request.Err)
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return 0, false
	}
	var network net.Error
	return 0, errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network)
}
