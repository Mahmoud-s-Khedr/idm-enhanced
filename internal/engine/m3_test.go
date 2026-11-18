package engine

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryAt struct {
	data []byte
	mu   sync.Mutex
}

func (w *memoryAt) WriteAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return copy(w.data[offset:], p), nil
}
func TestParallelWindows(t *testing.T) {
	for _, mode := range []string{"ok", "ignored", "etag", "range", "short", "long", "status", "cancel", "writer"} {
		t.Run(mode, func(t *testing.T) {
			var active atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				active.Add(1)
				defer active.Add(-1)
				var start, end int
				if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
					t.Error(err)
					return
				}
				if mode == "cancel" {
					<-r.Context().Done()
					return
				}
				if mode == "status" {
					w.WriteHeader(503)
					return
				}
				if mode == "ignored" {
					_, _ = io.WriteString(w, "abcdefgh")
					return
				}
				w.Header().Set("ETag", `"v1"`)
				if mode == "etag" {
					w.Header().Set("ETag", `"v2"`)
				}
				cr := fmt.Sprintf("bytes %d-%d/8", start, end)
				if mode == "range" {
					cr = "bytes 0-0/8"
				}
				w.Header().Set("Content-Range", cr)
				body := "abcdefgh"[start : end+1]
				if mode == "short" {
					w.Header().Set("Content-Length", fmt.Sprint(len(body)))
					body = body[:len(body)-1]
				}
				if mode == "long" {
					w.(http.Flusher).Flush()
					return
				} // separate custom-response length cases are covered below
				w.WriteHeader(206)
				_, _ = io.WriteString(w, body)
			}))
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "cancel" {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			dst := &memoryAt{data: make([]byte, 8)}
			var writer io.WriterAt = dst
			if mode == "writer" {
				writer = badWriterAt{}
			}
			end, err := Parallel(ctx, s.Client(), s.URL, Representation{Effective: s.URL, ETag: `"v1"`, Length: 8}, writer, 0, 2, 4)
			if mode == "ok" {
				if err != nil || end != 8 || string(dst.data) != "abcdefgh" {
					t.Fatal(end, err, string(dst.data))
				}
			} else if err == nil {
				t.Fatal("accepted", mode)
			}
			deadline := time.Now().Add(time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if active.Load() != 0 {
				t.Fatal("handlers remain")
			}
		})
	}
	for _, v := range []struct {
		workers      int
		start, chunk int64
	}{{0, 0, 1}, {17, 0, 1}, {1, -1, 1}, {1, 9, 1}, {1, 0, 0}} {
		if _, err := Parallel(context.Background(), http.DefaultClient, "", Representation{Length: 8}, &memoryAt{}, v.start, v.chunk, v.workers); err == nil {
			t.Fatal("invalid limits")
		}
	}
}

type badWriterAt struct{}

func (badWriterAt) WriteAt([]byte, int64) (int, error) { return 0, errors.New("disk") }
func TestBoundedRangeAndRetryClassification(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 206, Header: http.Header{"Etag": {`"v1"`}, "Content-Range": {"bytes 0-1/8"}}, ContentLength: -1, Body: io.NopCloser(strings.NewReader("toolong"))}, nil
	})}
	_, err := Stream(context.Background(), client, Request{Source: "http://test", Resume: true, End: 2, Identity: Representation{ETag: `"v1"`, Effective: "http://test", Length: 8}}, io.Discard, nil, nil)
	if !errors.Is(err, ErrUnsafeResume) {
		t.Fatal(err)
	}
	for _, end := range []int64{1, 2} {
		_, err = Stream(context.Background(), client, Request{Source: "http://test", Resume: true, Offset: 2, End: end, Identity: Representation{ETag: `"v1"`}}, io.Discard, nil, nil)
		if !errors.Is(err, ErrUnsafeResume) {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		err   error
		retry bool
	}{{io.ErrUnexpectedEOF, true}, {&url.Error{Op: "Get", URL: "http://fixture/?secret", Err: x509.UnknownAuthorityError{}}, false}, {&net.DNSError{IsNotFound: true}, false}, {errors.Join(io.ErrUnexpectedEOF, &transferError{"write", io.ErrUnexpectedEOF}), false}, {errors.Join(&HTTPError{503, time.Second}, io.ErrUnexpectedEOF), true}, {&net.DNSError{IsTemporary: true}, true}, {&transferError{"write", io.ErrUnexpectedEOF}, false}, {ErrUnsafeResume, false}, {context.Canceled, false}, {context.DeadlineExceeded, false}, {errors.New("disk"), false}, {&HTTPError{503, time.Second}, true}, {&HTTPError{401, 0}, false}, {&HTTPError{429, 0}, true}} {
		_, ok := RetryDelay(tc.err)
		if ok != tc.retry {
			t.Fatal(tc.err, ok)
		}
	}
	for _, value := range []string{"3", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat), "bad", "-1"} {
		r := &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": {value}}}
		err := statusError(r)
		delay, ok := RetryDelay(err)
		if !ok || (value == "3" && delay != 3*time.Second) {
			t.Fatal(value, delay, err)
		}
	}
}
func TestSharedLimits(t *testing.T) {
	var active, maximum atomic.Int32
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		defer active.Add(-1)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "x")
	}))
	defer s.Close()
	tr := &LimitedTransport{PerHost: 2, BytesPerSecond: 1024}
	client := &http.Client{Transport: tr}
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		})
	}
	deadline := time.Now().Add(time.Second)
	for active.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	if maximum.Load() > 2 || maximum.Load() == 0 {
		t.Fatal(maximum.Load())
	}
	tr.mu.Lock()
	if len(tr.hosts) != 0 {
		t.Error("host slots leaked")
	}
	tr.mu.Unlock()
	tr.CloseIdleConnections()
	// A queued request must cancel even while another body holds its host slot.
	held := &LimitedTransport{PerHost: 1, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "http://test", nil)
	resp, err := held.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(ctx)
	if unexpected, err := held.RoundTrip(req); !errors.Is(err, context.Canceled) {
		if unexpected != nil {
			_ = unexpected.Body.Close()
		}
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	_ = resp.Body.Close()
	broken := &LimitedTransport{PerHost: 1, Base: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })}
	if unexpected, err := broken.RoundTrip(req); err == nil {
		if unexpected != nil {
			_ = unexpected.Body.Close()
		}
		t.Fatal("transport failure")
	}
}
func TestBandwidthAndCancellation(t *testing.T) {
	tr := &LimitedTransport{BytesPerSecond: 327680, rate: make(chan struct{}, 1)}
	b := &limitedBody{ReadCloser: io.NopCloser(bytes.NewReader(make([]byte, 65536))), ctx: context.Background(), transport: tr}
	start := time.Now()
	if _, err := io.Copy(io.Discard, b); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("rate exceeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.ctx = ctx
	if _, err := b.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	tr.rate <- struct{}{}
	if _, err := b.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-tr.rate
	if err := wait(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBandwidthSharedAcrossBodies(t *testing.T) {
	tr := &LimitedTransport{BytesPerSecond: 655360, rate: make(chan struct{}, 1)}
	var wg sync.WaitGroup
	start := time.Now()
	for range 2 {
		wg.Go(func() {
			b := &limitedBody{ReadCloser: io.NopCloser(bytes.NewReader(make([]byte, 65536))), ctx: context.Background(), transport: tr}
			if _, err := io.Copy(io.Discard, b); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatal("independent rather than global bandwidth", elapsed)
	}
}

func TestParallelRequiresTransferEndpoints(t *testing.T) {
	if _, err := Parallel(context.Background(), nil, "", Representation{Length: 8}, &memoryAt{}, 0, 2, 4); err == nil {
		t.Fatal("nil client")
	}
	if _, err := Parallel(context.Background(), http.DefaultClient, "", Representation{Length: 8}, nil, 0, 2, 4); err == nil {
		t.Fatal("nil destination")
	}
}
