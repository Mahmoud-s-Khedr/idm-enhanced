// Package manager owns durable job lifecycle, recovery and retention policy.
package manager

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/localfs"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/storage"
)

type Job = storage.Job
type Options struct {
	URL, Destination, SHA256 string
	Progress                 func(int64, int64)
}
type Manager struct {
	store        *storage.Store
	client       *http.Client
	boundary     func(string) error
	mu           sync.Mutex
	active       map[string]context.CancelFunc
	config       Config
	queueRunning bool
}

func Open(dir string, readonly bool, client *http.Client) (*Manager, error) {
	return OpenContext(context.Background(), dir, readonly, client)
}

func OpenContext(ctx context.Context, dir string, readonly bool, client *http.Client) (*Manager, error) {
	s, err := storage.Open(dir, readonly)
	if err != nil {
		return nil, err
	}
	m := &Manager{store: s, active: make(map[string]context.CancelFunc)}
	if err := m.Configure(DefaultConfig(), client); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if !readonly {
		if err := m.recoverContext(ctx); err != nil {
			return nil, errors.Join(err, s.Close())
		}
	}
	return m, nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) > 0 || m.queueRunning {
		return errors.New("manager is busy")
	}
	m.client.CloseIdleConnections()
	return m.store.Close()
}
func (m *Manager) Status(ctx context.Context) ([]Job, error) { return m.store.List(ctx) }

func (m *Manager) Create(ctx context.Context, o Options) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs, err := m.store.List(ctx)
	if err != nil {
		return Job{}, err
	}
	pending := 0
	for _, j := range jobs {
		if j.State != "completed" && j.State != "cancelled" {
			pending++
		}
	}
	if pending >= m.config.QueueLimit {
		return Job{}, errors.New("queue limit reached")
	}
	if o.Destination == "" {
		return Job{}, errors.New("destination is required")
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Job{}, errors.New("invalid HTTP URL")
	}
	sum, err := hex.DecodeString(o.SHA256)
	if err != nil || (o.SHA256 != "" && len(sum) != 32) {
		return Job{}, errors.New("SHA-256 must contain 64 hexadecimal characters")
	}
	dest, err := filepath.Abs(o.Destination)
	if err != nil {
		return Job{}, err
	}
	parent, parentErr := os.Stat(filepath.Dir(dest))
	if parentErr != nil || !parent.IsDir() {
		return Job{}, errors.New("destination parent must be an existing directory")
	}
	if err := vacant(dest); err != nil {
		return Job{}, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Job{}, err
	}
	j := Job{Version: 1, ID: hex.EncodeToString(id), Source: o.URL, Destination: dest, Checksum: o.SHA256, Length: -1, State: "queued", Created: time.Now().UTC()}
	for _, existing := range jobs {
		j.QueueOrder = max(j.QueueOrder, existing.QueueOrder+1)
	}
	if err := m.store.Save(ctx, j); err != nil {
		return Job{}, err
	}
	return j, nil
}
func vacant(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

var transitions = map[string]string{
	"queued":          " inspecting cancelled needs_attention",
	"inspecting":      " downloading paused needs_attention failed cancelled",
	"downloading":     " pausing verifying paused needs_attention failed cancelled",
	"pausing":         " paused failed needs_attention",
	"paused":          " inspecting queued cancelled needs_attention verifying",
	"needs_attention": " queued cancelled verifying",
	"verifying":       " completed needs_attention failed paused",
	"completed":       "",
	"failed":          " inspecting queued cancelled needs_attention paused retry_wait",
	"retry_wait":      " inspecting queued paused cancelled needs_attention failed",
	"cancelled":       " inspecting queued needs_attention verifying",
}

func (m *Manager) transition(ctx context.Context, j *Job, state string) error {
	if j.State != state && !strings.Contains(transitions[j.State], " "+state) {
		return errors.New("illegal job transition")
	}
	next := *j
	next.State = state
	if err := m.store.Save(ctx, next); err != nil {
		return err
	}
	*j = next
	return nil
}
func owned(j Job, flag int) (*os.File, error) {
	if j.Partial == "" || j.Identity == "" {
		return nil, errors.New("partial ownership unavailable; explicit restart required")
	}
	info, err := os.Lstat(j.Partial)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("owned partial missing or replaced; explicit restart required")
	}
	f, err := os.OpenFile(j.Partial, flag, 0)
	if err != nil {
		return nil, err
	}
	identity, err := localfs.Identity(f)
	ownershipErr := localfs.VerifyOwnership(f, j.Ownership)
	if err != nil || identity != j.Identity || ownershipErr != nil {
		return nil, errors.Join(errors.New("partial identity changed; explicit restart required"), err, ownershipErr, f.Close())
	}
	return f, nil
}
func digest(reader io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.CopyBuffer(h, reader, make([]byte, 32*1024))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (m *Manager) allocate(ctx context.Context, j *Job) (*os.File, error) {
	if err := vacant(j.Destination); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(j.Destination), "."+filepath.Base(j.Destination)+"-*.part")
	if err != nil {
		return nil, err
	}
	j.Partial = f.Name()
	j.Ownership = rand.Text()
	j.Identity, err = localfs.Identity(f)
	if err == nil {
		err = localfs.Protect(f.Name(), false)
	}
	if err == nil {
		err = localfs.TagOwnership(f, j.Ownership)
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = localfs.SyncDir(filepath.Dir(j.Partial))
	}
	if err == nil {
		err = m.store.Save(ctx, *j)
	}
	if err != nil {
		return nil, errors.Join(errors.New("partial allocation unrecorded; retained for inspection"), err, f.Close())
	}
	return f, nil
}

// reconcile persists the longest proven prefix before removing uncertain bytes.
func (m *Manager) reconcile(ctx context.Context, j *Job, f *os.File) error {
	var end int64
	valid := 0
	for _, interval := range j.Intervals {
		if interval.Start != end || interval.End <= end {
			break
		}
		sum, err := digest(contextReader{ctx: ctx, reader: io.NewSectionReader(f, end, interval.End-end)})
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.Size() < interval.End || sum != interval.Digest {
			break
		}
		end = interval.End
		valid++
	}
	next := *j
	next.Intervals = next.Intervals[:valid]
	next.Durable = end
	next.Generation++
	if err := m.store.Save(ctx, next); err != nil {
		return err
	}
	*j = next
	if err := f.Truncate(end); err != nil {
		return err
	}
	_, err := f.Seek(end, io.SeekStart)
	return err
}
func (m *Manager) recover() error { return m.recoverContext(context.Background()) }
func (m *Manager) recoverContext(ctx context.Context) error {
	jobs, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.State == "completed" {
			continue
		}
		if j.Phase == "verified" && j.State != "cancelled" {
			if err := m.publish(ctx, &j); err != nil {
				j.Error = "publication_recovery"
				if e := m.transition(ctx, &j, "needs_attention"); e != nil {
					return e
				}
			}
			continue
		}
		if j.Partial != "" {
			f, e := owned(j, os.O_RDWR)
			if e == nil {
				e = m.reconcile(ctx, &j, f)
				e = errors.Join(e, f.Close())
			}
			if e != nil {
				j.Error = "local_identity"
				if e = m.transition(ctx, &j, "needs_attention"); e != nil {
					return e
				}
				continue
			}
		}
		if j.State == "inspecting" || j.State == "downloading" || j.State == "pausing" || j.State == "verifying" || j.State == "retry_wait" {
			if err := m.transition(ctx, &j, "paused"); err != nil {
				return err
			}
		}
	}
	return nil
}

type checkpointWriter struct {
	file     *os.File
	manager  *Manager
	job      *Job
	position int64
	last     time.Time
	ctx      context.Context
}

func (w *checkpointWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	w.position += int64(n)
	if err == nil && (w.position-w.job.Durable >= 4*1024*1024 || time.Since(w.last) >= time.Second) {
		err = w.checkpoint(w.ctx)
	}
	return n, err
}
func (w *checkpointWriter) checkpoint(ctx context.Context) error {
	if w.position == w.job.Durable {
		return nil
	}
	if err := w.manager.step("before-file-sync"); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	if err := w.manager.step("after-file-sync"); err != nil {
		return err
	}
	sum, err := digest(io.NewSectionReader(w.file, w.job.Durable, w.position-w.job.Durable))
	if err != nil {
		return err
	}
	next := *w.job
	next.Intervals = append(append([]storage.Interval(nil), next.Intervals...), storage.Interval{Start: next.Durable, End: w.position, Digest: sum})
	next.Durable = w.position
	next.Generation++
	if err := w.manager.store.Save(ctx, next); err != nil {
		return err
	}
	*w.job = next
	w.last = time.Now()
	return w.manager.step("after-checkpoint")
}

// runAttempt uses the existing recovery and finalization path for one attempt.
func (m *Manager) runAttempt(ctx context.Context, id string, restart bool, progress func(int64, int64), attempt int) (err error) {
	j, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.State == "completed" {
		return nil
	}
	j.RetryAt = time.Time{}
	if !restart && j.Phase == "verified" && j.State == "cancelled" {
		if err := m.transition(ctx, &j, "verifying"); err != nil {
			return err
		}
		return m.publish(ctx, &j)
	}
	if restart {
		if err := m.reset(ctx, &j); err != nil {
			return err
		}
	}
	j.RetryCount = attempt
	if j.State == "needs_attention" || j.Error == "discarded" {
		return engine.ErrUnsafeResume
	}
	if err := m.transition(ctx, &j, "inspecting"); err != nil {
		return err
	}
	var f *os.File
	if j.Partial == "" {
		f, err = m.allocate(ctx, &j)
	} else {
		f, err = owned(j, os.O_RDWR)
		if err == nil {
			err = m.reconcile(ctx, &j, f)
		}
	}
	if err != nil {
		if f != nil {
			err = errors.Join(err, f.Close())
		}
		return m.fail(&j, err, "local_identity", true)
	}
	defer func() {
		if f != nil {
			err = errors.Join(err, f.Close())
		}
	}()
	identity := engine.Representation{Effective: j.Effective, ETag: j.ETag, Encoding: j.Encoding, Length: j.Length}
	resumed := j.Effective != ""
	if resumed {
		// Empty representations cannot produce a valid 0-0 range; verify by full GET.
		if j.Length == 0 {
			if !engine.StrongETag(j.ETag) {
				return m.fail(&j, engine.ErrUnsafeResume, "remote_identity", true)
			}
			resumed = false
		} else {
			probe := engine.Request{Source: j.Source, Resume: true, Probe: true, Identity: identity}
			var rep engine.Representation
			rep, err = engine.Stream(ctx, m.client, probe, io.Discard, nil, nil)
			if err != nil {
				return m.fail(&j, err, "remote_identity", errors.Is(err, engine.ErrUnsafeResume))
			}
			j.Length = rep.Length
			identity.Length = rep.Length
		}
	}
	if err = m.transition(ctx, &j, "downloading"); err != nil {
		return err
	}
	writer := &checkpointWriter{file: f, manager: m, job: &j, position: j.Durable, last: time.Now(), ctx: ctx}
	if !resumed || j.Durable != j.Length {
		_, err = m.stream(ctx, &j, identity, resumed, writer, func(n, total int64) {
			if progress != nil {
				progress(writer.position, total)
			}
		}, func(rep engine.Representation) error {
			if identity.Effective != "" && identity.Length == 0 && (rep.Length != 0 || rep.Effective != identity.Effective || rep.ETag != identity.ETag) {
				return engine.ErrUnsafeResume
			}
			j.Effective = rep.Effective
			j.ETag = rep.ETag
			j.Encoding = rep.Encoding
			j.Length = rep.Length
			return m.store.Save(ctx, j)
		})
	}
	cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err != nil {
		// Only accepted bytes are checkpointed. Unsafe response bodies remain uncertain.
		if !errors.Is(err, engine.ErrUnsafeResume) {
			err = errors.Join(err, writer.checkpoint(cleanup))
		}
		return m.fail(&j, err, "transfer", errors.Is(err, engine.ErrUnsafeResume))
	}
	if err = writer.checkpoint(cleanup); err != nil {
		return m.fail(&j, err, "checkpoint", false)
	}
	if err = ctx.Err(); err != nil {
		return m.fail(&j, err, "interrupted", false)
	}
	if err = m.transition(ctx, &j, "verifying"); err != nil {
		return err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	j.Digest, err = digest(contextReader{ctx: ctx, reader: f})
	if err != nil {
		return m.fail(&j, err, "verification", false)
	}
	if j.Checksum != "" && !strings.EqualFold(j.Digest, j.Checksum) {
		return m.fail(&j, errors.New("SHA-256 mismatch"), "checksum", true)
	}
	j.Length = writer.position
	if err = f.Sync(); err != nil {
		return m.fail(&j, err, "sync", false)
	}
	err = f.Close()
	f = nil
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return m.fail(&j, err, "interrupted", false)
	}
	j.Phase = "verified"
	if err = m.step("before-verified-commit"); err != nil {
		return err
	}
	if err = m.store.Save(ctx, j); err != nil {
		return err
	}
	if err = m.step("after-verified-commit"); err != nil {
		return err
	}
	if err = m.publish(ctx, &j); err != nil {
		return m.fail(&j, err, "publication", true)
	}
	return nil
}
func (m *Manager) fail(j *Job, cause error, category string, attention bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state := "failed"
	if attention {
		state = "needs_attention"
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		state = "paused"
	}
	j.Error = category
	if state == "paused" && j.State == "downloading" {
		if err := m.transition(ctx, j, "pausing"); err != nil {
			return errors.Join(cause, err)
		}
	}
	return errors.Join(cause, m.transition(ctx, j, state))
}
func verifyFile(ctx context.Context, path string, j Job) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("publication file missing or replaced")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	identity, e := localfs.Identity(f)
	ownershipErr := localfs.VerifyOwnership(f, j.Ownership)
	sum, se := digest(contextReader{ctx: ctx, reader: f})
	err = errors.Join(e, ownershipErr, se, f.Close())
	if err != nil {
		return err
	}
	if identity != j.Identity || info.Size() != j.Length || sum != j.Digest {
		return errors.New("publication evidence mismatch")
	}
	return nil
}
func (m *Manager) publish(ctx context.Context, j *Job) error {
	// Validate and persist the recovery transition before publishing any files.
	if j.State != "verifying" {
		if err := m.transition(ctx, j, "verifying"); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(j.Destination); errors.Is(err, os.ErrNotExist) {
		if err := verifyFile(ctx, j.Partial, *j); err != nil {
			return err
		}
		if err := m.step("before-publish"); err != nil {
			return err
		}
		if err := os.Link(j.Partial, j.Destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := m.step("after-publish"); err != nil {
		return err
	}
	if err := verifyFile(ctx, j.Destination, *j); err != nil {
		return err
	}
	if err := localfs.SyncDir(filepath.Dir(j.Destination)); err != nil {
		return err
	}
	if _, err := os.Lstat(j.Partial); err == nil {
		f, err := owned(*j, os.O_RDONLY)
		if err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = os.Remove(j.Partial); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := localfs.SyncDir(filepath.Dir(j.Destination)); err != nil {
		return err
	}
	if err := m.step("after-remove"); err != nil {
		return err
	}
	j.Error = ""
	if err := m.transition(ctx, j, "completed"); err != nil {
		return err
	}
	return m.step("after-completed")
}
func (m *Manager) discard(j *Job) error {
	if j.Partial == "" {
		return nil
	}
	f, err := owned(*j, os.O_RDONLY)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Remove(j.Partial); err != nil {
		return err
	}
	return localfs.SyncDir(filepath.Dir(j.Partial))
}
func (m *Manager) reset(ctx context.Context, j *Job) error {
	if j.Partial != "" {
		if _, err := os.Lstat(j.Partial); err == nil {
			if f, ownershipErr := owned(*j, os.O_RDONLY); ownershipErr == nil {
				if err := f.Close(); err != nil {
					return err
				}
				if err := m.discard(j); err != nil {
					return err
				}
			} // Replaced files are left untouched; explicit restart abandons the old record.
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	j.Partial = ""
	j.Identity = ""
	j.Ownership = ""
	j.Effective = ""
	j.ETag = ""
	j.Encoding = ""
	j.Length = -1
	j.Durable = 0
	j.Intervals = nil
	j.Generation++
	j.Phase = ""
	j.Digest = ""
	j.Error = ""
	j.RetryCount = 0
	j.RetryAt = time.Time{}
	return m.transition(ctx, j, "queued")
}
func (m *Manager) Cancel(ctx context.Context, id string, keep bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[id]; ok || m.queueRunning {
		return errors.New("pause active job before cancellation")
	}
	j, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.State == "completed" {
		return nil
	}
	if !keep && j.Partial != "" {
		if err := m.discard(&j); err != nil {
			return err
		}
		j.Partial = ""
		j.Identity = ""
		j.Ownership = ""
		j.Durable = 0
		j.Intervals = nil
		j.Error = "discarded"
		j.Phase = ""
		j.Digest = ""
	}
	return m.transition(ctx, &j, "cancelled")
}
func Resumable(j Job) bool {
	return j.State != "completed" && j.State != "needs_attention" && j.Partial != "" && engine.StrongETag(j.ETag)
}

// SafeError is the persisted category; diagnostic text never includes source URLs.
func SafeError(j Job) string { return j.Error }

// step provides deterministic fault boundaries for recovery tests. It is unset in adapters.
func (m *Manager) step(name string) error {
	if m.boundary != nil {
		return m.boundary(name)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
