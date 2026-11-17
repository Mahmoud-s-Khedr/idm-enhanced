package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type testBody struct {
	io.Reader
	closeErr error
	closed   bool
}

func (b *testBody) Close() error { b.closed = true; return b.closeErr }

type failingWriter struct{ short bool }

func (w failingWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("disk full")
}

func TestTransferFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		writer   io.Writer
		length   int64
		encoding string
		closeErr error
	}{
		{name: "write", writer: failingWriter{}, length: 5},
		{name: "short write", writer: failingWriter{short: true}, length: 5},
		{name: "wrong length", writer: io.Discard, length: 10},
		{name: "encoding", writer: io.Discard, length: 5, encoding: "gzip"},
		{name: "close", writer: io.Discard, length: 5, closeErr: errors.New("close error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &testBody{Reader: strings.NewReader("hello"), closeErr: tc.closeErr}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: body, ContentLength: tc.length, Header: http.Header{"Content-Encoding": []string{tc.encoding}}}, nil
			})}
			if err := Transfer(context.Background(), client, "https://example.invalid", tc.writer, nil); err == nil {
				t.Fatal("expected failure")
			}
			if !body.closed {
				t.Fatal("response leaked")
			}
		})
	}
	for _, source := range []string{"file:///tmp/x", "https://user:password@example.invalid", "http://%zz", "http://"} {
		if err := Transfer(context.Background(), http.DefaultClient, source, io.Discard, nil); err == nil {
			t.Fatal("accepted invalid URL")
		}
	}
	if err := Transfer(context.Background(), nil, "http://example.invalid", io.Discard, nil); err == nil {
		t.Fatal("accepted nil client")
	}
}

func TestTransferRedirectLimitAndRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/?token=secret", http.StatusFound) }))
	defer server.Close()
	err := Transfer(context.Background(), server.Client(), server.URL+"/?token=secret", io.Discard, nil)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestTransferMidBodyCancellation(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "timeout"}[timeout], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "started")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if timeout {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 100*time.Millisecond)
				defer stop()
			}
			started := time.Now()
			err := Transfer(ctx, server.Client(), server.URL, io.Discard, func(n, _ int64) {
				if n > 0 && !timeout {
					cancel()
				}
			})
			want := context.Canceled
			if timeout {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || time.Since(started) > 3*time.Second {
				t.Fatalf("cancellation: %v", err)
			}
		})
	}
}

// A generated 64 MiB response never materializes in memory; the sink rejects any
// write larger than the transfer budget. No io.Copy fast path can bypass it.
type generatedBody struct{ remaining int64 }

func (b *generatedBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > b.remaining {
		n = int(b.remaining)
	}
	clear(p[:n])
	b.remaining -= int64(n)
	return n, nil
}
func (*generatedBody) Close() error { return nil }

type boundedSink struct{ bytes int64 }

func (s *boundedSink) Write(p []byte) (int, error) {
	if len(p) > 32*1024 {
		return 0, errors.New("transfer buffer exceeded")
	}
	s.bytes += int64(len(p))
	return len(p), nil
}
func TestTransferBoundedBuffer(t *testing.T) {
	const size = 64 * 1024 * 1024
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &generatedBody{size}, ContentLength: size, Header: make(http.Header)}, nil
	})}
	sink := new(boundedSink)
	if err := Transfer(context.Background(), client, "https://example.invalid", sink, nil); err != nil {
		t.Fatal(err)
	}
	if sink.bytes != size {
		t.Fatalf("transferred %d", sink.bytes)
	}
}
func BenchmarkTransferStreaming(b *testing.B) {
	const size = 64 * 1024 * 1024
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: &generatedBody{size}, ContentLength: size, Header: make(http.Header)}, nil
	})}
	b.ReportAllocs()
	b.SetBytes(size)
	for b.Loop() {
		if err := Transfer(context.Background(), client, "https://example.invalid", io.Discard, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestTransferRejectsUnsafeRedirect(t *testing.T) {
	for _, location := range []string{"http://example.invalid/file", "https://user:password@example.invalid/file", "file:///tmp/file"} {
		t.Run(location, func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{location}}}, nil
			})}
			err := Transfer(context.Background(), client, "https://example.invalid/start", io.Discard, nil)
			if err == nil || requests != 1 || strings.Contains(err.Error(), "password") {
				t.Fatalf("redirect: %d %v", requests, err)
			}
		})
	}
}
