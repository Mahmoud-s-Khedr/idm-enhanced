package manager

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
)

type Config struct {
	Workers, Jobs, PerHost, QueueLimit, Retries int
	BytesPerSecond                              int64
	Backoff                                     time.Duration
}

func DefaultConfig() Config {
	return Config{Workers: 4, Jobs: 2, PerHost: 4, QueueLimit: 1000, Retries: 3, Backoff: time.Second}
}

// Configure is allowed only while idle. Limits apply to all requests using this
// manager, including sequential transfers, probes and cross-host redirects.
func (m *Manager) Configure(c Config, client *http.Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || len(m.active) > 0 || m.queueRunning || len(m.analysisSlots) > 0 {
		return errors.New("manager is busy")
	}
	if c.Workers < 1 || c.Workers > 16 || c.Jobs < 1 || c.Jobs > 16 || c.PerHost < 1 || c.PerHost > 64 || c.QueueLimit < 1 || c.QueueLimit > 10000 || c.Retries < 0 || c.Retries > 8 || c.BytesPerSecond < 0 || c.BytesPerSecond > 1<<40 || c.Backoff <= 0 || c.Backoff > time.Minute {
		return errors.New("invalid manager limits")
	}
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.Transport = &engine.LimitedTransport{Base: client.Transport, PerHost: c.PerHost, BytesPerSecond: c.BytesPerSecond}
	m.client = &copyClient
	m.config = c
	return nil
}

// Run owns a job for its complete retry lifecycle. Concurrent commands for the
// same job are rejected, and a caller cannot exceed the manager's active limit.
func (m *Manager) Run(ctx context.Context, id string, restart bool, progress func(int64, int64)) error {
	return m.run(ctx, id, restart, progress, false)
}
func (m *Manager) run(ctx context.Context, id string, restart bool, progress func(int64, int64), queued bool) error {
	m.mu.Lock()
	if _, ok := m.active[id]; ok || len(m.active) >= m.config.Jobs || (m.queueRunning && !queued) {
		m.mu.Unlock()
		return errors.New("job or manager is busy")
	}
	ctx, cancel := context.WithCancel(ctx)
	m.active[id] = cancel
	m.mu.Unlock()
	defer func() { cancel(); m.mu.Lock(); delete(m.active, id); m.mu.Unlock() }()
	saved, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !restart && !saved.RetryAt.IsZero() {
		if err := waitRetry(ctx, time.Until(saved.RetryAt)); err != nil {
			return m.fail(&saved, err, "interrupted", false)
		}
	}
	for attempt := 0; ; attempt++ {
		err := m.runAttempt(ctx, id, restart, progress, attempt)
		delay, retry := engine.RetryDelay(err)
		if err == nil || !retry {
			return err
		}
		// A no-validator stream cannot be resumed. It can be restarted safely only
		// for this automatic retry of a transient failure, retaining ownership checks.
		j, getErr := m.store.Get(ctx, id)
		if getErr != nil {
			return errors.Join(err, getErr)
		}
		if j.State != "failed" {
			return err
		}
		restart = j.Effective != "" && !engine.StrongETag(j.ETag)
		serverDelay := delay
		delay = max(delay, m.config.Backoff*time.Duration(1<<attempt))
		// Exhausting the automatic budget must not discard a server's outstanding
		// Retry-After. Explicit resume/relaunch still honors its persisted deadline.
		if attempt >= m.config.Retries || delay > 5*time.Minute {
			if serverDelay > 0 {
				j.RetryAt = time.Now().Add(serverDelay).UTC()
				return errors.Join(err, m.store.Save(ctx, j))
			}
			return err
		}
		j.RetryCount = attempt + 1
		j.RetryAt = time.Now().Add(delay).UTC()
		if saveErr := m.transition(ctx, &j, "retry_wait"); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		if waitErr := waitRetry(ctx, delay); waitErr != nil {
			return m.fail(&j, waitErr, "interrupted", false)
		}
	}
}
func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Pause propagates cancellation; Run/RunQueue returns only after workers and
// checkpoint cleanup finish. Callers wait for that return before Cancel/Close.
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cancel, ok := m.active[id]
	if !ok {
		return errors.New("job is not active")
	}
	cancel()
	return nil
}

func queueJobs(jobs []Job) []Job {
	queue := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if j.State == "queued" {
			queue = append(queue, j)
		}
	}
	sort.SliceStable(queue, func(i, j int) bool {
		a, b := queue[i], queue[j]
		if a.QueueOrder != b.QueueOrder {
			return a.QueueOrder < b.QueueOrder
		}
		if !a.Created.Equal(b.Created) {
			return a.Created.Before(b.Created)
		}
		return a.ID < b.ID
	})
	return queue
}

// Reorder atomically replaces the complete order of currently queued jobs.
func (m *Manager) Reorder(ctx context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) > 0 || m.queueRunning {
		return errors.New("manager is busy")
	}
	jobs, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	queue := queueJobs(jobs)
	if len(ids) != len(queue) {
		return errors.New("provide every queued job exactly once")
	}
	byID := make(map[string]Job, len(queue))
	for _, j := range queue {
		byID[j.ID] = j
	}
	ordered := make([]Job, 0, len(ids))
	for i, id := range ids {
		j, ok := byID[id]
		if !ok {
			return errors.New("invalid queue order")
		}
		delete(byID, id)
		j.QueueOrder = int64(i + 1)
		ordered = append(ordered, j)
	}
	return m.store.SaveBatch(ctx, ordered)
}

// RunQueue drains a bounded snapshot of queued jobs. New enqueues are left for
// the next invocation. One failed job does not cancel unrelated queued jobs.
func (m *Manager) RunQueue(ctx context.Context) error {
	return m.runQueue(ctx, nil)
}

func (m *Manager) runQueue(ctx context.Context, completed func(string)) error {
	m.mu.Lock()
	if m.queueRunning || len(m.active) > 0 {
		m.mu.Unlock()
		return errors.New("manager is busy")
	}
	m.queueRunning = true
	jobs, err := m.store.List(ctx)
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.queueRunning = false; m.mu.Unlock() }()
	if err != nil {
		return err
	}
	queue := queueJobs(jobs)
	due := queue[:0]
	for _, j := range queue {
		if j.ScheduledAt.IsZero() || !j.ScheduledAt.After(time.Now()) {
			due = append(due, j)
		}
	}
	queue = due
	results := make(chan error, m.config.Jobs)
	next, active := 0, 0
	var failure error
	for next < len(queue) || active > 0 {
		for ctx.Err() == nil && next < len(queue) && active < m.config.Jobs {
			id := queue[next].ID
			next++
			active++
			go func() {
				err := m.run(ctx, id, false, nil, true)
				if err == nil && completed != nil {
					completed(id)
				}
				results <- err
			}()
		}
		if active == 0 {
			break
		}
		failure = errors.Join(failure, <-results)
		active--
	}
	return errors.Join(failure, ctx.Err())
}

// Schedule persists an absolute UTC start instant. Zero clears the schedule.
func (m *Manager) Schedule(ctx context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) > 0 || m.queueRunning {
		return errors.New("manager is busy")
	}
	j, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.State != "queued" {
		return errors.New("only queued jobs can be scheduled")
	}
	j.ScheduledAt = at.UTC()
	return m.store.Save(ctx, j)
}
func (m *Manager) PauseQueued(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) > 0 || m.queueRunning {
		return errors.New("manager is busy")
	}
	j, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if j.State != "queued" {
		return errors.New("job is not queued")
	}
	return m.transition(ctx, &j, "paused")
}

// Remove never deletes a completed file. Unfinished jobs must be cancelled first.
func (m *Manager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.active) > 0 || m.queueRunning {
		return errors.New("manager is busy")
	}
	j, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if (j.State != "completed" && j.State != "cancelled") || (j.State != "completed" && j.Partial != "") {
		return errors.New("cancel and discard partial data before removing unfinished history")
	}
	if j.Kind == "hls" || j.Kind == "dash" {
		if err := m.discardMedia(&j); err != nil {
			return err
		}
	}
	return m.store.Delete(ctx, id)
}
