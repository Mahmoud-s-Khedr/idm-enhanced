package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LimitedTransport applies one shared per-host request bound and body-byte rate
// across jobs, probes and redirects. Host slots are retained until body Close.
// Bandwidth uses serialized 32 KiB reservations: at most one chunk of burst.
type LimitedTransport struct {
	Base           http.RoundTripper
	PerHost        int
	BytesPerSecond int64
	mu             sync.Mutex
	hosts          map[string]int
	changed        chan struct{}
	rate           chan struct{}
	next           time.Time
}

func (t *LimitedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	host := strings.ToLower(strings.TrimSuffix(r.URL.Hostname(), "."))
	t.mu.Lock()
	if t.hosts == nil {
		t.hosts = make(map[string]int)
		t.changed = make(chan struct{})
		t.rate = make(chan struct{}, 1)
	}
	for t.PerHost > 0 && t.hosts[host] >= t.PerHost {
		changed := t.changed
		t.mu.Unlock()
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-changed:
		}
		t.mu.Lock()
	}
	t.hosts[host]++
	t.mu.Unlock()
	release := func() {
		t.mu.Lock()
		t.hosts[host]--
		if t.hosts[host] == 0 {
			delete(t.hosts, host)
		}
		close(t.changed)
		t.changed = make(chan struct{})
		t.mu.Unlock()
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(r)
	if err != nil {
		release()
		return nil, err
	}
	resp.Body = &limitedBody{ReadCloser: resp.Body, ctx: r.Context(), transport: t, release: release}
	return resp, nil
}
func (t *LimitedTransport) CloseIdleConnections() {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if c, ok := base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

type limitedBody struct {
	io.ReadCloser
	ctx       context.Context
	transport *LimitedTransport
	release   func()
	once      sync.Once
}

func (b *limitedBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
func (b *limitedBody) Read(p []byte) (int, error) {
	t := b.transport
	if t.BytesPerSecond <= 0 {
		return b.ReadCloser.Read(p)
	}
	select {
	case t.rate <- struct{}{}:
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
	defer func() { <-t.rate }()
	if err := wait(b.ctx, time.Until(t.next)); err != nil {
		return 0, err
	}
	n, err := b.ReadCloser.Read(p[:min(len(p), 32*1024)])
	t.next = time.Now().Add(time.Duration(float64(n) / float64(t.BytesPerSecond) * float64(time.Second)))
	return n, err
}
func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
