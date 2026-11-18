package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func m3Manager(t *testing.T, client *http.Client, c Config) *Manager {
	t.Helper()
	m, err := Open(t.TempDir(), false, client)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Configure(c, client); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m
}
func m3Job(t *testing.T, m *Manager, source string) Job {
	t.Helper()
	j, err := m.Create(context.Background(), Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func TestSegmentedManager(t *testing.T) {
	data := bytes.Repeat([]byte("range-data-"), 900000)
	for _, mode := range []string{"ok", "ignored probe", "changed worker", "short worker", "cancel", "checkpoint", "weak", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			var ranges, active, maximum atomic.Int32
			ready := make(chan struct{})
			var once sync.Once
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"v1"`)
				if mode == "weak" {
					w.Header().Set("ETag", `W/"v1"`)
				}
				if r.Header.Get("Range") == "bytes=0-0" {
					if mode == "ignored probe" {
						w.Header().Set("Content-Length", fmt.Sprint(len(data)))
						_, _ = w.Write(data)
						return
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(data)))
					w.WriteHeader(206)
					_, _ = w.Write(data[:1])
					return
				}
				if value := r.Header.Get("Range"); value != "" {
					ranges.Add(1)
					n := active.Add(1)
					defer active.Add(-1)
					for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
					}
					var start, end int
					end = len(data) - 1
					if _, err := fmt.Sscanf(value, "bytes=%d-%d", &start, &end); err != nil {
						_, _ = fmt.Sscanf(value, "bytes=%d-", &start)
					}
					if mode == "ok" {
						if n >= 2 {
							once.Do(func() { close(ready) })
						}
						select {
						case <-ready:
						case <-r.Context().Done():
							return
						}
					}
					if mode == "cancel" {
						<-r.Context().Done()
						return
					}
					if mode == "changed worker" {
						w.Header().Set("ETag", `"v2"`)
					}
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
					w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
					w.WriteHeader(206)
					if mode == "short worker" {
						_, _ = w.Write(data[start : start+1])
						return
					}
					// A small delay gives other workers time to issue their disjoint requests.
					time.Sleep(5 * time.Millisecond)
					_, _ = w.Write(data[start : end+1])
					return
				}
				if mode == "unknown" {
					w.(http.Flusher).Flush()
				} else {
					w.Header().Set("Content-Length", fmt.Sprint(len(data)))
				}
				_, _ = w.Write(data)
			}))
			defer s.Close()
			c := DefaultConfig()
			c.Retries = 0
			m := m3Manager(t, s.Client(), c)
			j := m3Job(t, m, s.URL)
			if mode == "checkpoint" {
				m.boundary = func(name string) error {
					if name == "after-file-sync" {
						return errors.New("sync fault")
					}
					return nil
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if mode == "cancel" {
				ctx, cancel = context.WithTimeout(ctx, 80*time.Millisecond)
				defer cancel()
			}
			err := m.Run(ctx, j.ID, false, nil)
			fail := mode == "changed worker" || mode == "short worker" || mode == "cancel" || mode == "checkpoint"
			if (err != nil) != fail {
				t.Fatal(mode, err)
			}
			saved, getErr := m.store.Get(context.Background(), j.ID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if !fail {
				actual, readErr := os.ReadFile(j.Destination)
				if readErr != nil || !bytes.Equal(actual, data) {
					t.Fatal("incorrect output", readErr)
				}
				if mode == "ok" && (ranges.Load() < 2 || maximum.Load() < 2) {
					t.Fatal("not parallel", ranges.Load(), maximum.Load())
				}
				return
			}
			if _, statErr := os.Stat(j.Destination); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("published failure")
			}
			if saved.Durable != 0 {
				t.Fatal("acknowledged speculative bytes", saved.Durable)
			}
			deadline := time.Now().Add(time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if active.Load() != 0 {
				t.Fatal("active handlers")
			}
			if mode == "changed worker" && saved.State != "needs_attention" {
				t.Fatal(saved.State)
			}
		})
	}
}
func TestQueueOrderingAndLimits(t *testing.T) {
	var active, maximum atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		time.Sleep(10 * time.Millisecond)
		if r.URL.Path == "/bad" {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, "hello")
	}))
	defer s.Close()
	c := DefaultConfig()
	c.Jobs = 2
	c.PerHost = 1
	c.QueueLimit = 3
	c.Retries = 0
	m := m3Manager(t, s.Client(), c)
	a, b, d := m3Job(t, m, s.URL), m3Job(t, m, s.URL+"/bad"), m3Job(t, m, s.URL)
	if _, err := m.Create(context.Background(), Options{}); err == nil {
		t.Fatal("unbounded queue")
	}
	for _, ids := range [][]string{{a.ID}, {a.ID, b.ID, b.ID}, {a.ID, b.ID, "missing"}} {
		if err := m.Reorder(context.Background(), ids); err == nil {
			t.Fatal("bad reorder")
		}
	}
	if err := m.Reorder(context.Background(), []string{d.ID, a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.Status(context.Background())
	queue := queueJobs(jobs)
	if queue[0].ID != d.ID {
		t.Fatal("wrong order")
	}
	if err := m.RunQueue(context.Background()); err == nil {
		t.Fatal("queue suppressed a failed job")
	}
	if maximum.Load() != 1 {
		t.Fatal("per-host bound", maximum.Load())
	}
	for _, j := range []Job{a, d} {
		got, _ := m.store.Get(context.Background(), j.ID)
		if got.State != "completed" {
			t.Fatal(got.State)
		}
	}
	if err := m.RunQueue(context.Background()); err != nil {
		t.Fatal("empty queue", err)
	}
	e := m3Job(t, m, s.URL)
	if e.QueueOrder <= b.QueueOrder {
		t.Fatal("enqueue lost order")
	}
}
func TestActiveCommandsAndQueueCancel(t *testing.T) {
	started := make(chan struct{}, 4)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }))
	defer s.Close()
	c := DefaultConfig()
	c.Jobs = 1
	m := m3Manager(t, s.Client(), c)
	a := m3Job(t, m, s.URL)
	b := m3Job(t, m, s.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- m.Run(ctx, a.ID, false, nil) }()
	<-started
	if err := m.Run(ctx, a.ID, false, nil); err == nil {
		t.Fatal("same job ran twice")
	}
	if err := m.Run(ctx, b.ID, false, nil); err == nil {
		t.Fatal("job limit bypassed")
	}
	if err := m.Close(); err == nil {
		t.Fatal("closed busy manager")
	}
	if err := m.Configure(c, nil); err == nil {
		t.Fatal("configured busy manager")
	}
	if err := m.Cancel(ctx, a.ID, false); err == nil {
		t.Fatal("cancelled without joining")
	}
	if err := m.Reorder(ctx, []string{b.ID}); err == nil {
		t.Fatal("reordered active queue")
	}
	if err := m.RunQueue(ctx); err == nil {
		t.Fatal("overlapping queue")
	}
	if err := m.Pause(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := m.Pause("missing"); err == nil {
		t.Fatal("paused missing")
	}
	if err := m.Cancel(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	queueCtx, stop := context.WithCancel(ctx)
	go func() { result <- m.RunQueue(queueCtx) }()
	<-started
	if err := m.RunQueue(ctx); err == nil {
		t.Fatal("overlapping queue")
	}
	if err := m.Run(ctx, a.ID, false, nil); err == nil {
		t.Fatal("external run entered queue")
	}
	if err := m.Cancel(ctx, b.ID, true); err == nil {
		t.Fatal("mutated queue snapshot")
	}
	stop()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cancelled, stopNow := context.WithCancel(ctx)
	stopNow()
	if err := m.RunQueue(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestRetries(t *testing.T) {
	for _, mode := range []string{"success", "exhausted", "permanent", "long wait", "cancel wait", "nonvalidator", "disk", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if mode != "nonvalidator" {
					w.Header().Set("ETag", `"v1"`)
				}
				if mode == "truncated" || mode == "nonvalidator" {
					if n == 1 {
						w.Header().Set("Content-Length", "5")
						_, _ = io.WriteString(w, "he")
						return
					}
					if r.Header.Get("Range") == "bytes=0-0" {
						w.Header().Set("Content-Range", "bytes 0-0/5")
						w.WriteHeader(206)
						_, _ = io.WriteString(w, "h")
						return
					}
					if r.Header.Get("Range") == "bytes=2-" {
						w.Header().Set("Content-Range", "bytes 2-4/5")
						w.WriteHeader(206)
						_, _ = io.WriteString(w, "llo")
						return
					}
				}
				if mode == "permanent" {
					w.WriteHeader(401)
					return
				}
				if mode == "long wait" {
					w.Header().Set("Retry-After", "600")
					w.WriteHeader(429)
					return
				}
				if mode == "cancel wait" {
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(503)
					return
				}
				if mode == "exhausted" || n == 1 && mode == "success" {
					w.WriteHeader(503)
					return
				}
				_, _ = io.WriteString(w, "hello")
			}))
			defer s.Close()
			c := DefaultConfig()
			c.Retries = 2
			c.Backoff = time.Millisecond
			m := m3Manager(t, s.Client(), c)
			j := m3Job(t, m, s.URL)
			if mode == "disk" {
				m.boundary = func(name string) error {
					if name == "before-file-sync" {
						return errors.New("disk")
					}
					return nil
				}
			}
			timeout := time.Second
			if mode == "cancel wait" {
				timeout = 5 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			var err error
			if mode == "cancel wait" {
				err = cancelDuringRetry(t, ctx, cancel, m, j.ID)
			} else {
				err = m.Run(ctx, j.ID, false, nil)
			}
			success := mode == "success" || mode == "nonvalidator" || mode == "truncated"
			if (err == nil) != success {
				t.Fatal(mode, err)
			}
			if mode == "exhausted" && calls.Load() != 3 {
				t.Fatal("unbounded retries", calls.Load())
			}
			if (mode == "permanent" || mode == "long wait" || mode == "cancel wait" || mode == "disk") && calls.Load() != 1 {
				t.Fatal("retried unsafe failure", calls.Load())
			}
			if success {
				b, err := os.ReadFile(j.Destination)
				if err != nil || string(b) != "hello" {
					t.Fatal("bad retry output", err)
				}
			}
			saved, _ := m.store.Get(context.Background(), j.ID)
			if mode == "cancel wait" && saved.State != "paused" {
				t.Fatal(saved.State)
			}
		})
	}
}
func TestConfigAndClosedOperations(t *testing.T) {
	m, _ := fixture(t)
	for _, change := range []func(*Config){func(c *Config) { c.Workers = 0 }, func(c *Config) { c.Workers = 17 }, func(c *Config) { c.Jobs = 0 }, func(c *Config) { c.Jobs = 17 }, func(c *Config) { c.PerHost = 0 }, func(c *Config) { c.QueueLimit = 0 }, func(c *Config) { c.Retries = -1 }, func(c *Config) { c.Retries = 9 }, func(c *Config) { c.BytesPerSecond = -1 }, func(c *Config) { c.Backoff = 0 }} {
		c := DefaultConfig()
		change(&c)
		if err := m.Configure(c, nil); err == nil {
			t.Fatal("invalid config")
		}
	}
	if err := m.Run(context.Background(), "missing", false, nil); err == nil {
		t.Fatal("missing job")
	}
	if err := m.Reorder(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), Options{}); err == nil {
		t.Fatal("closed create")
	}
	if err := m.Reorder(context.Background(), nil); err == nil {
		t.Fatal("closed reorder")
	}
	if err := m.RunQueue(context.Background()); err == nil {
		t.Fatal("closed queue")
	}
}
func TestParallelPrefixRecovery(t *testing.T) {
	// Crash/reopen reconciliation discards uncommitted speculative ranges while
	// retaining the exact digest-verified prefix from an earlier successful window.
	m, source := fixture(t)
	j := partial(t, m, source)
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt([]byte("speculative"), rangeChunk); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err = m.recover(); err != nil {
		t.Fatal(err)
	}
	saved, _ := m.store.Get(context.Background(), j.ID)
	info, err := os.Stat(j.Partial)
	if err != nil || info.Size() != 2 || saved.Durable != 2 {
		t.Fatal("speculative tail survived", saved, err)
	}
	if err = m.Run(context.Background(), j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(j.Destination)
	if fmt.Sprintf("%x", sha256.Sum256(b)) != fmt.Sprintf("%x", sha256.Sum256([]byte("hello"))) {
		t.Fatal("recovery mismatch")
	}
}

func TestPersistentQueueAndRetryWait(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	var order []string
	var orderMu sync.Mutex
	retryRequest := make(chan time.Time, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/retry" {
			retryRequest <- time.Now()
		}
		orderMu.Lock()
		order = append(order, r.URL.Path)
		orderMu.Unlock()
		_, _ = io.WriteString(w, "hello")
	}))
	defer s.Close()
	m, err := Open(dir, false, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	a, b := m3Job(t, m, s.URL+"/a"), m3Job(t, m, s.URL+"/b")
	if err = m.Reorder(ctx, []string{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = Open(dir, false, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	c := DefaultConfig()
	c.Jobs = 1
	if err = m.Configure(c, s.Client()); err != nil {
		t.Fatal(err)
	}
	if err = m.RunQueue(ctx); err != nil {
		t.Fatal(err)
	}
	orderMu.Lock()
	recordedOrder := fmt.Sprint(order)
	orderMu.Unlock()
	if recordedOrder != "[/b /a]" {
		t.Fatal(recordedOrder)
	}
	j := m3Job(t, m, s.URL+"/retry")
	j.State = "retry_wait"
	j.RetryAt = time.Now().Add(80 * time.Millisecond)
	j.RetryCount = 1
	if err = m.store.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err = m.recover(); err != nil {
		t.Fatal(err)
	}
	got, _ := m.store.Get(ctx, j.ID)
	if got.State != "paused" || !got.RetryAt.Equal(j.RetryAt) {
		t.Fatal("lost retry wait", got)
	}
	if err = m.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	// Database sync/recovery may consume the remaining wait, especially on
	// Windows. The contract is no HTTP request before the saved deadline, not
	// a minimum delay measured after setup has already used part of that budget.
	select {
	case requested := <-retryRequest:
		if requested.Before(j.RetryAt) {
			t.Fatal("HTTP request preceded persisted Retry-After", requested, j.RetryAt)
		}
	default:
		t.Fatal("retry never reached fixture")
	}
}
func TestGlobalJobLimit(t *testing.T) {
	var active, maximum atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, "hello")
	}))
	defer s.Close()
	c := DefaultConfig()
	c.Jobs = 2
	c.PerHost = 8
	m := m3Manager(t, s.Client(), c)
	for range 5 {
		m3Job(t, m, s.URL)
	}
	if err := m.RunQueue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 {
		t.Fatal("global job limit", maximum.Load())
	}
}

type inspectionBody struct{ io.Reader }

func (inspectionBody) Close() error { return errors.New("close failed") }
func TestInspectionCloseFailure(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: managerTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, ContentLength: rangeChunk + 1, Header: http.Header{"Etag": {`"v1"`}}, Body: inspectionBody{bytes.NewReader(nil)}}, nil
	})}
	m := m3Manager(t, client, DefaultConfig())
	j := m3Job(t, m, "http://fixture")
	if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
		t.Fatal("ignored inspection close error")
	}
	if calls.Load() != 1 {
		t.Fatal("started workers after close failure", calls.Load())
	}
	if _, err := os.Stat(j.Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published after close failure")
	}
}

func TestExhaustedBudgetPreservesRetryAfter(t *testing.T) {
	for _, retries := range []int{0, 1} {
		t.Run(fmt.Sprint(retries), func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n > int32(retries) {
					w.Header().Set("Retry-After", "60")
				}
				w.WriteHeader(503)
			}))
			defer s.Close()
			dir := t.TempDir()
			m, err := Open(dir, false, s.Client())
			if err != nil {
				t.Fatal(err)
			}
			c := DefaultConfig()
			c.Retries = retries
			c.Backoff = time.Millisecond
			if err = m.Configure(c, s.Client()); err != nil {
				t.Fatal(err)
			}
			j := m3Job(t, m, s.URL)
			if err = m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("accepted exhausted failure")
			}
			saved, err := m.store.Get(context.Background(), j.ID)
			if err != nil || time.Until(saved.RetryAt) < 30*time.Second {
				t.Fatal("lost outstanding Retry-After", saved.RetryAt, err)
			}
			if calls.Load() != int32(retries+1) {
				t.Fatal("retry count", calls.Load())
			}
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			m, err = Open(dir, false, s.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err = m.Run(ctx, j.ID, false, nil); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("resume bypassed wait", err)
			}
			if calls.Load() != int32(retries+1) {
				t.Fatal("resume issued an early request", calls.Load())
			}
		})
	}
}

// Wait for the behavior under test, not a wall-clock guess that may expire
// before SQLite allocation or the first HTTP request on a loaded native runner.
func cancelDuringRetry(t *testing.T, ctx context.Context, cancel context.CancelFunc, m *Manager, id string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, id, false, nil) }()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		saved, err := m.store.Get(ctx, id)
		if err == nil && saved.State == "retry_wait" {
			cancel()
			result := <-done
			if !errors.Is(result, context.Canceled) {
				t.Fatalf("retry wait did not propagate cancellation: %v", result)
			}
			return result
		}
		select {
		case result := <-done:
			t.Fatalf("run ended before persisted retry wait: %v", result)
			return result
		case <-ctx.Done():
			cancel()
			result := <-done
			t.Fatalf("never reached retry wait: %v", result)
			return result
		case <-ticker.C:
		}
	}
}
