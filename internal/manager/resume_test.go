package manager

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/storage"
)

func fixture(t *testing.T) (*Manager, string) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if value := r.Header.Get("Range"); value != "" {
			var start int
			if value == "bytes=0-0" {
				w.Header().Set("Content-Range", "bytes 0-0/5")
				w.WriteHeader(206)
				_, _ = io.WriteString(w, "h")
				return
			}
			_, _ = fmt.Sscanf(value, "bytes=%d-", &start)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-4/5", start))
			w.WriteHeader(206)
			_, _ = io.WriteString(w, "hello"[start:])
			return
		}
		_, _ = io.WriteString(w, "hello")
	}))
	t.Cleanup(s.Close)
	m, err := Open(t.TempDir(), false, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, s.URL
}
func partial(t *testing.T, m *Manager, source string) Job {
	t.Helper()
	ctx := context.Background()
	j, err := m.Create(ctx, Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")})
	if err != nil {
		t.Fatal(err)
	}
	f, err := m.allocate(ctx, &j)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("heTAIL"); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	j.Effective = source
	j.ETag = `"v1"`
	j.Length = 5
	j.State = "paused"
	j.Durable = 2
	j.Generation = 1
	j.Intervals = []storage.Interval{{Start: 0, End: 2, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("he")))}}
	if err = m.store.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	return j
}
func TestResumeAndRecovery(t *testing.T) {
	for _, damage := range []string{"none", "modified", "truncated", "missing", "replaced", "weak", "unknown", "complete"} {
		t.Run(damage, func(t *testing.T) {
			m, source := fixture(t)
			j := partial(t, m, source)
			switch damage {
			case "modified":
				if err := os.WriteFile(j.Partial, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			case "truncated":
				if err := os.Truncate(j.Partial, 1); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(j.Partial); err != nil {
					t.Fatal(err)
				}
			case "replaced":
				if err := os.Rename(j.Partial, j.Partial+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(j.Partial, []byte("unrelated"), 0600); err != nil {
					t.Fatal(err)
				}
			case "weak":
				j.ETag = `W/"v1"`
			case "unknown":
				j.Length = -1
			case "complete":
				if err := os.WriteFile(j.Partial, []byte("hello"), 0600); err != nil {
					t.Fatal(err)
				}
				j.Durable = 5
				j.Intervals = []storage.Interval{{Start: 0, End: 5, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))}}
			}
			j.State = "downloading"
			if err := m.store.Save(context.Background(), j); err != nil {
				t.Fatal(err)
			}
			if err := m.recover(); err != nil {
				t.Fatal(err)
			}
			jobs, err := m.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if jobs[0].State == "downloading" {
				t.Fatal("abandoned running job")
			}
			err = m.Run(context.Background(), j.ID, false, nil)
			unsafe := damage == "missing" || damage == "replaced" || damage == "weak"
			if (err != nil) != unsafe {
				t.Fatal(err)
			}
			if !unsafe {
				b, err := os.ReadFile(j.Destination)
				if err != nil || string(b) != "hello" {
					t.Fatal(string(b), err)
				}
			}
			if damage == "replaced" {
				b, err := os.ReadFile(j.Partial)
				if err != nil || string(b) != "unrelated" {
					t.Fatal("touched unrelated partial")
				}
			}
			if damage == "weak" || damage == "missing" {
				if err := m.Run(context.Background(), j.ID, true, nil); err != nil {
					t.Fatal("restart", err)
				}
			}
		})
	}
}
func TestRetention(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(fmt.Sprint(keep), func(t *testing.T) {
			m, source := fixture(t)
			j := partial(t, m, source)
			ctx := context.Background()
			if err := m.Cancel(ctx, j.ID, keep); err != nil {
				t.Fatal(err)
			}
			if err := m.Cancel(ctx, j.ID, keep); err != nil {
				t.Fatal(err)
			}
			err := m.Run(ctx, j.ID, false, nil)
			if keep {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("discarded job resumed without restart")
				}
				if err = m.Run(ctx, j.ID, true, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err = m.Cancel(ctx, j.ID, false); err != nil {
				t.Fatal(err)
			}
			if err = m.Run(ctx, j.ID, true, nil); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(j.Destination)
			if err != nil || string(b) != "hello" {
				t.Fatal("completed file changed")
			}
		})
	}
}
func TestPause(t *testing.T) {
	m, source := fixture(t)
	j := partial(t, m, source)
	ctx, cancel := context.WithCancel(context.Background())
	err := m.Run(ctx, j.ID, false, func(n, _ int64) {
		if n > 2 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	saved, err := m.store.Get(context.Background(), j.ID)
	if err != nil || saved.State != "paused" || saved.Durable != 5 {
		t.Fatal(saved, err)
	}
	if err = m.Run(context.Background(), j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
}
func TestPublicationRecovery(t *testing.T) {
	for _, phase := range []string{"verified", "linked", "removed", "collision", "modified"} {
		t.Run(phase, func(t *testing.T) {
			m, source := fixture(t)
			j := partial(t, m, source)
			if err := os.WriteFile(j.Partial, []byte("hello"), 0600); err != nil {
				t.Fatal(err)
			}
			j.Length = 5
			j.Digest = fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))
			j.Phase = "verified"
			j.State = "verifying"
			if phase == "linked" || phase == "removed" {
				if err := os.Link(j.Partial, j.Destination); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "removed" {
				if err := os.Remove(j.Partial); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "collision" {
				if err := os.WriteFile(j.Destination, []byte("other"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "modified" {
				if err := os.WriteFile(j.Partial, []byte("wrong"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := m.store.Save(context.Background(), j); err != nil {
				t.Fatal(err)
			}
			if err := m.recover(); err != nil {
				t.Fatal(err)
			}
			got, err := m.store.Get(context.Background(), j.ID)
			if err != nil {
				t.Fatal(err)
			}
			expected := "completed"
			if phase == "collision" || phase == "modified" {
				expected = "needs_attention"
			}
			if got.State != expected {
				t.Fatal(got.State)
			}
			if err := m.recover(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestManagerBoundaries(t *testing.T) {
	m, source := fixture(t)
	ctx := context.Background()
	for _, o := range []Options{{}, {URL: "file:///tmp/a", Destination: "x"}, {URL: source, Destination: "x", SHA256: "bad"}} {
		if _, err := m.Create(ctx, o); err == nil {
			t.Fatal("accepted invalid options")
		}
	}
	if err := m.Run(ctx, "missing", false, nil); err == nil {
		t.Fatal("missing job")
	}
	if err := m.Cancel(ctx, "missing", false); err == nil {
		t.Fatal("missing cancel")
	}
	j := partial(t, m, source)
	j.State = "completed"
	if err := m.transition(ctx, &j, "downloading"); err == nil {
		t.Fatal("illegal transition")
	}
	if Resumable(j) {
		t.Fatal("completed resumable")
	}
	if SafeError(j) != "" {
		t.Fatal("unsafe status")
	}
	if f, err := owned(Job{}, os.O_RDONLY); err == nil {
		_ = f.Close()
		t.Fatal("adopted unowned")
	}
	if err := verifyFile(ctx, j.Partial, Job{Identity: j.Identity, Length: 6, Digest: "bad"}); err == nil {
		t.Fatal("invalid publication")
	}
	if err := verifyFile(ctx, j.Partial+"missing", j); err == nil {
		t.Fatal("missing publication")
	}
	if err := m.discard(&Job{}); err != nil {
		t.Fatal(err)
	}
	j.State = "paused"
	j.Intervals[0].Start = 1
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.reconcile(ctx, &j, f); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if j.Durable != 0 {
		t.Fatal("invalid interval accepted")
	}
	if _, err := digest(errorReader{}); err == nil {
		t.Fatal("ignored digest read error")
	}
	// Fail a checkpoint sync without advancing metadata.
	f, err = owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	w := checkpointWriter{file: f, manager: m, job: &j, ctx: ctx}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	w.position = 10
	if err = w.checkpoint(ctx); err == nil || j.Durable != 0 {
		t.Fatal("acknowledged failed sync")
	}
	if _, err = w.Write([]byte("data")); err == nil {
		t.Fatal("ignored write error")
	}
	// A rejected checkpoint transaction preserves the durable generation.
	f, err = owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w.file = f
	w.position = 1
	if _, err = f.WriteString("x"); err != nil {
		t.Fatal(err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if err = w.checkpoint(cancelled); err == nil || j.Durable != 0 {
		t.Fatal("advanced failed transaction")
	}
	if err = m.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Status(ctx); err == nil {
		t.Fatal("closed status")
	}
	if err = m.transition(ctx, &j, "inspecting"); err == nil {
		t.Fatal("closed transition")
	}
	if err = m.recover(); err == nil {
		t.Fatal("closed recovery")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }
func TestRemoteChanges(t *testing.T) {
	for _, response := range []string{"changed", "expired", "weak", "ignored"} {
		t.Run(response, func(t *testing.T) {
			m, source := fixture(t)
			j := partial(t, m, source)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", `"v2"`)
				if response == "expired" {
					w.WriteHeader(403)
					return
				}
				if response == "weak" {
					w.Header().Set("ETag", `W/"v1"`)
				}
				if response != "ignored" {
					w.Header().Set("Content-Range", "bytes 0-0/5")
					w.WriteHeader(206)
				}
				_, _ = io.WriteString(w, "h")
			}))
			defer s.Close()
			j.Source = s.URL
			j.Effective = s.URL
			if err := m.store.Save(context.Background(), j); err != nil {
				t.Fatal(err)
			}
			if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("accepted unsafe source")
			}
			b, err := os.ReadFile(j.Partial)
			if err != nil || string(b) != "he" {
				t.Fatal("changed durable prefix", string(b), err)
			}
			jobs, err := m.Status(context.Background())
			if err != nil || jobs[0].State != "needs_attention" {
				t.Fatal(jobs, err)
			}
			if strings.Contains(SafeError(jobs[0]), s.URL) {
				t.Fatal("source leak")
			}
		})
	}
}
func TestCheckpointAndPublicationFaults(t *testing.T) {
	for _, boundary := range []string{"before-file-sync", "after-file-sync", "after-checkpoint", "before-verified-commit", "after-verified-commit", "before-publish", "after-publish", "after-remove", "after-completed"} {
		t.Run(boundary, func(t *testing.T) {
			m, source := fixture(t)
			j, err := m.Create(context.Background(), Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")})
			if err != nil {
				t.Fatal(err)
			}
			m.boundary = func(step string) error {
				if step == boundary {
					return errors.New("injected I/O failure")
				}
				return nil
			}
			if err = m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("ignored fault")
			}
			saved, err := m.store.Get(context.Background(), j.ID)
			if err != nil {
				t.Fatal(err)
			}
			if (boundary == "before-file-sync" || boundary == "after-file-sync") && saved.Durable != 0 {
				t.Fatal("acknowledged uncommitted bytes")
			}
			m.boundary = nil
			if err = m.recover(); err != nil {
				t.Fatal(err)
			}
			if err = m.Run(context.Background(), j.ID, false, nil); err != nil {
				t.Fatal("failed recovery", err)
			}
			data, err := os.ReadFile(j.Destination)
			if err != nil || string(data) != "hello" {
				t.Fatal(string(data), err)
			}
		})
	}
}
func TestEmptyAndTimedCheckpoint(t *testing.T) {
	m, source := fixture(t)
	j := partial(t, m, source)
	j.Length = 0
	j.Intervals = nil
	j.Durable = 0
	if err := os.Truncate(j.Partial, 0); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.Header().Set("ETag", `"v1"`)
	}))
	defer server.Close()
	j.Source = server.URL
	j.Effective = server.URL
	if err := m.store.Save(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	j = partial(t, m, source)
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err = f.Seek(j.Durable, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	writer := checkpointWriter{file: f, manager: m, job: &j, ctx: context.Background(), position: j.Durable}
	if _, err = writer.Write([]byte("llo")); err != nil {
		t.Fatal(err)
	}
	if j.Durable != 5 {
		t.Fatal("time checkpoint did not commit")
	}
}
func TestUnavailableStorageAndOwnership(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing"), true, http.DefaultClient); err == nil {
		t.Fatal("missing store")
	}
	m, source := fixture(t)
	j := partial(t, m, source)
	ctx := context.Background()
	original := j.Partial
	if err := os.Rename(j.Partial, j.Partial+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(j.Partial, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(ctx, j.ID, false); err == nil {
		t.Fatal("deleted replaced partial")
	}
	if err := m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("accepted replaced partial")
	}
	if err := m.Run(ctx, j.ID, true, nil); err != nil {
		t.Fatal("restart should leave replacement untouched", err)
	}
	if info, err := os.Stat(original); err != nil || !info.IsDir() {
		t.Fatal("removed unrelated directory")
	}
	j, err := m.Create(ctx, Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Dir(j.Destination)); err != nil {
		t.Fatal(err)
	}
	if err = m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("missing output parent")
	}
	j = partial(t, m, source)
	if err = m.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Create(ctx, Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")}); err == nil {
		t.Fatal("closed creation")
	}
	if _, err = m.allocate(ctx, &j); err == nil {
		t.Fatal("unrecorded allocation")
	}
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err = m.reconcile(ctx, &j, f); err == nil {
		t.Fatal("rewound without metadata commit")
	}
}
func TestFilesystemFailureRecovery(t *testing.T) {
	m, source := fixture(t)
	ctx := context.Background()
	j := partial(t, m, source)
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = m.reconcile(ctx, &j, f); err == nil {
		t.Fatal("ignored checkpoint read error")
	}
	f, err = owned(j, os.O_RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.reconcile(ctx, &j, f); err == nil {
		t.Fatal("ignored truncate failure")
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = owned(j, os.O_WRONLY)
	if err != nil {
		t.Fatal(err)
	}
	writer := checkpointWriter{file: f, manager: m, job: &j, ctx: ctx, position: j.Durable + 1}
	if err = writer.checkpoint(ctx); err == nil {
		t.Fatal("ignored checkpoint digest failure")
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = m.Create(ctx, Options{URL: source, Destination: filepath.Join(t.TempDir(), "result")})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(j.Destination, []byte("collision"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("accepted destination collision before allocation")
	}
}
func TestInvalidParentAndRestart(t *testing.T) {
	m, source := fixture(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, Options{URL: source, Destination: filepath.Join(file, "child")}); err == nil {
		t.Fatal("accepted non-directory parent")
	}
	j := partial(t, m, source)
	j.Partial = filepath.Join(file, "child")
	if err := m.store.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(ctx, j.ID, true, nil); err == nil {
		t.Fatal("ignored partial inspection error")
	}
}
func TestEmptyResumeIdentityAndVerificationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := digest(contextReader{ctx: ctx, reader: strings.NewReader("data")}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, etag := range []string{`W/"v1"`, `"v1"`} {
		t.Run(etag, func(t *testing.T) {
			m, source := fixture(t)
			j := partial(t, m, source)
			j.Length = 0
			j.Durable = 0
			j.Intervals = nil
			j.ETag = etag
			if err := os.Truncate(j.Partial, 0); err != nil {
				t.Fatal(err)
			}
			if err := m.store.Save(context.Background(), j); err != nil {
				t.Fatal(err)
			}
			if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("empty source changed or validator unsafe")
			}
		})
	}
}
func TestCancelledVerifiedJob(t *testing.T) {
	m, source := fixture(t)
	j := partial(t, m, source)
	ctx := context.Background()
	if err := os.WriteFile(j.Partial, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	j.Phase = "verified"
	j.State = "needs_attention"
	j.Length = 5
	j.Durable = 5
	j.Digest = fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))
	j.Intervals = []storage.Interval{{Start: 0, End: 5, Digest: j.Digest}}
	if err := m.store.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(ctx, j.ID, true); err != nil {
		t.Fatal(err)
	}
	// Cancellation must not be undone by unrelated modifying commands.
	if err := m.recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(j.Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published cancelled job")
	}
	if err := m.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal("explicit resume of verified cancelled job", err)
	}
}

// Hashing never asks the source for a buffer larger than the transfer budget.
type boundedHashReader struct{ remaining int64 }

func (r *boundedHashReader) Read(p []byte) (int, error) {
	if len(p) > 32*1024 {
		return 0, errors.New("checksum buffer exceeded")
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	clear(p[:n])
	r.remaining -= int64(n)
	return n, nil
}
func TestBoundedChecksum(t *testing.T) {
	for _, size := range []int64{0, 32 * 1024, 64 * 1024 * 1024} {
		if _, err := digest(&boundedHashReader{size}); err != nil {
			t.Fatal(err)
		}
	}
}
func BenchmarkChecksum(b *testing.B) {
	for _, size := range []int64{1024 * 1024, 64 * 1024 * 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(size)
			for b.Loop() {
				if _, err := digest(&boundedHashReader{size}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func TestLongestValidPrefixAndByteCheckpoint(t *testing.T) {
	m, source := fixture(t)
	j := partial(t, m, source)
	ctx := context.Background()
	j.Intervals = append(j.Intervals, storage.Interval{Start: 2, End: 5, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("llo")))})
	j.Durable = 5
	if err := os.WriteFile(j.Partial, []byte("heBADuncertain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.store.Save(ctx, j); err != nil {
		t.Fatal(err)
	}
	f, err := owned(j, os.O_RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err = m.reconcile(ctx, &j, f); err != nil {
		t.Fatal(err)
	}
	if j.Durable != 2 || len(j.Intervals) != 1 {
		t.Fatal("did not retain longest valid prefix", j.Durable)
	}
	writer := checkpointWriter{file: f, manager: m, job: &j, ctx: ctx, position: j.Durable, last: time.Now()}
	block := make([]byte, 32*1024)
	for range 128 {
		if _, err = writer.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if j.Durable != 2+4*1024*1024 {
		t.Fatal("4 MiB checkpoint missing")
	}
}
func TestRepeatedPauseReleasesRequests(t *testing.T) {
	done := make(chan struct{}, 16)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		done <- struct{}{}
	}))
	defer s.Close()
	m, err := Open(t.TempDir(), false, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	for range 16 {
		ctx, cancel := context.WithCancel(context.Background())
		j, err := m.Create(ctx, Options{URL: s.URL, Destination: filepath.Join(t.TempDir(), "result")})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		err = m.Run(ctx, j.ID, false, func(n, _ int64) {
			if n > 0 {
				cancel()
			}
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("request leaked after pause")
		}
	}
}
func TestCancelledStartupReleasesOwnership(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenContext(ctx, dir, false, http.DefaultClient); !errors.Is(err, context.Canceled) {
		t.Fatal("startup did not honor cancellation", err)
	}
	m, err := Open(dir, false, http.DefaultClient)
	if err != nil {
		t.Fatal("startup leaked process ownership", err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
}
