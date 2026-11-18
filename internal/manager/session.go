package manager

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Session owns the application's asynchronous runs and schedule clock. All job
// work still passes through Manager.Run/RunQueue and the existing recovery path.
// commands serializes admission, pause/join, configuration and shutdown.
type Session struct {
	manager          *Manager
	commands         sync.Mutex
	mu               sync.Mutex
	cancel           context.CancelFunc
	done             chan struct{}
	runID            string
	beforeRun        func(context.Context) // Test synchronization before asynchronous manager admission.
	closed, sleeping bool
	schedulingPaused bool
	failure          string
	settings         Settings
	clockCancel      context.CancelFunc
	clockDone        chan struct{}
	notify           func(Job)
	analysisContext  context.Context
	analysisCancel   context.CancelFunc
	analyses         sync.WaitGroup
}
type Settings struct {
	Workers       int   `json:"workers"`
	Jobs          int   `json:"jobs"`
	PerHost       int   `json:"perHost"`
	Retries       int   `json:"retries"`
	Rate          int64 `json:"rate"`
	Notifications bool  `json:"notifications"`
}

func NewSession(m *Manager, notify func(Job)) (*Session, error) {
	defaults := DefaultConfig()
	s := &Session{manager: m, notify: notify, settings: Settings{Workers: defaults.Workers, Jobs: defaults.Jobs, PerHost: defaults.PerHost, Retries: defaults.Retries, Rate: defaults.BytesPerSecond, Notifications: true}, clockDone: make(chan struct{})}
	b, err := m.store.Preferences(context.Background())
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err = json.Unmarshal(b, &s.settings); err != nil {
			return nil, errors.New("invalid saved settings")
		}
	}
	if err = m.Configure(s.settings.config(), nil); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.analysisContext, s.analysisCancel = context.WithCancel(context.Background())
	s.clockCancel = cancel
	go func() {
		defer close(s.clockDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.tick(now)
			}
		}
	}()
	return s, nil
}
func (p Settings) config() Config {
	c := DefaultConfig()
	c.Workers = p.Workers
	c.Jobs = p.Jobs
	c.PerHost = p.PerHost
	c.Retries = p.Retries
	c.BytesPerSecond = p.Rate
	return c
}
func (s *Session) Settings() Settings { s.mu.Lock(); defer s.mu.Unlock(); return s.settings }
func (s *Session) State() (bool, bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done != nil, s.sleeping, s.failure
}
func (s *Session) available() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sleeping {
		return errors.New("application is stopping or sleeping")
	}
	return nil
}
func (s *Session) Configure(p Settings) error {
	s.commands.Lock()
	defer s.commands.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	s.mu.Lock()
	busy := s.done != nil
	old := s.settings
	s.mu.Unlock()
	if busy {
		return errors.New("pause all downloads before changing limits")
	}
	if err := s.manager.Configure(p.config(), nil); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err == nil {
		err = s.manager.store.SavePreferences(context.Background(), b)
	}
	if err != nil {
		return errors.Join(err, s.manager.Configure(old.config(), nil))
	}
	s.mu.Lock()
	s.settings = p
	s.mu.Unlock()
	return nil
}
func (s *Session) Create(o Options) (Job, error) {
	s.commands.Lock()
	defer s.commands.Unlock()
	if err := s.available(); err != nil {
		return Job{}, err
	}
	return s.manager.Create(context.Background(), o)
}
func (s *Session) Start(id string, restart bool) error {
	s.commands.Lock()
	defer s.commands.Unlock()
	err := s.start(id, restart)
	if err == nil {
		s.mu.Lock()
		s.schedulingPaused = false
		s.mu.Unlock()
	}
	return err
}
func (s *Session) start(id string, restart bool) error {
	if err := s.available(); err != nil {
		return err
	}
	// Validate before acknowledging asynchronous admission.
	wasCompleted := false
	if id != "" {
		j, err := s.manager.store.Get(context.Background(), id)
		if err != nil {
			return err
		}
		wasCompleted = j.State == "completed"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		return errors.New("pause all downloads before starting another run")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done
	s.runID = id
	s.failure = ""
	go func() {
		if s.beforeRun != nil {
			s.beforeRun(ctx)
		}
		var err error
		if id == "" {
			err = s.manager.runQueue(ctx, s.completed)
		} else {
			err = s.manager.Run(ctx, id, restart, nil)
			if err == nil && !wasCompleted {
				s.completed(id)
			}
		}
		s.mu.Lock()
		if err != nil && !errors.Is(err, context.Canceled) {
			s.failure = "Some downloads need attention. Check their status and retry."
		}
		s.mu.Unlock()
		cancel()
		s.mu.Lock()
		s.cancel = nil
		s.done = nil
		s.runID = ""
		close(done)
		s.mu.Unlock()
	}()
	return nil
}

// completed runs after verified finalization, before the worker is joined.
func (s *Session) completed(id string) {
	s.mu.Lock()
	enabled := s.settings.Notifications
	s.mu.Unlock()
	if !enabled || s.notify == nil {
		return
	}
	j, err := s.manager.store.Get(context.Background(), id)
	if err != nil {
		s.mu.Lock()
		s.failure = "Unable to read download status."
		s.mu.Unlock()
		return
	}
	if j.State == "completed" {
		s.notify(j)
	}
}
func (s *Session) stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}
func (s *Session) PauseAll() {
	s.commands.Lock()
	defer s.commands.Unlock()
	s.mu.Lock()
	s.schedulingPaused = true
	s.mu.Unlock()
	s.analysisCancel()
	s.stop()
	s.analyses.Wait()
	s.mu.Lock()
	if !s.closed && !s.sleeping {
		s.analysisContext, s.analysisCancel = context.WithCancel(context.Background())
	}
	s.mu.Unlock()
}
func (s *Session) Command(action, id string, keep bool, at time.Time, order []string) error {
	s.commands.Lock()
	defer s.commands.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	s.mu.Lock()
	busy := s.done != nil
	individual := busy && s.runID == id && id != ""
	s.mu.Unlock()
	if busy && action != "pause" {
		return errors.New("pause all downloads before changing jobs")
	}
	ctx := context.Background()
	switch action {
	case "pause":
		if individual {
			// Cancel and join even when the run is reserved but not yet active.
			s.stop()
			j, err := s.manager.store.Get(ctx, id)
			if err != nil || j.State == "paused" {
				return err
			}
			return s.manager.PauseQueued(ctx, id)
		}
		if err := s.manager.Pause(id); err == nil {
			return nil
		}
		return s.manager.PauseQueued(ctx, id)
	case "cancel":
		return s.manager.Cancel(ctx, id, keep)
	case "remove":
		return s.manager.Remove(ctx, id)
	case "schedule":
		return s.manager.Schedule(ctx, id, at)
	case "reorder":
		return s.manager.Reorder(ctx, order)
	default:
		return errors.New("unknown download command")
	}
}

// Sleep stops admission and joins active work. Wake permits due scheduled jobs;
// interrupted transfers remain paused and require explicit resume.
func (s *Session) Sleep() {
	s.commands.Lock()
	defer s.commands.Unlock()
	s.mu.Lock()
	s.sleeping = true
	s.mu.Unlock()
	s.analysisCancel()
	s.stop()
	s.analyses.Wait()
}
func (s *Session) Wake() {
	s.commands.Lock()
	defer s.commands.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sleeping = false
	if !s.closed && s.analysisContext.Err() != nil {
		s.analysisContext, s.analysisCancel = context.WithCancel(context.Background())
	}
}
func (s *Session) tick(now time.Time) {
	s.commands.Lock()
	defer s.commands.Unlock()
	s.mu.Lock()
	blocked := s.closed || s.sleeping || s.schedulingPaused || s.done != nil
	s.mu.Unlock()
	if blocked {
		return
	}
	jobs, err := s.manager.Status(context.Background())
	if err != nil {
		s.mu.Lock()
		s.failure = "Unable to read scheduled downloads."
		s.mu.Unlock()
		return
	}
	for _, j := range queueJobs(jobs) {
		if !j.ScheduledAt.IsZero() && !j.ScheduledAt.After(now) {
			if err := s.start(j.ID, false); err != nil {
				s.mu.Lock()
				s.failure = "Unable to start scheduled download."
				s.mu.Unlock()
			}
			return
		}
	}
}

// Close rejects new commands, stops the clock, checkpoints and joins workers,
// then releases the database owner. It is safe to call repeatedly.
func (s *Session) Close() error {
	s.commands.Lock()
	defer s.commands.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.clockCancel()
	s.analysisCancel()
	// The clock may be waiting for commands; join it outside that lock.
	s.stop()
	s.analyses.Wait()
	err := s.manager.Close()
	s.commands.Unlock()
	<-s.clockDone
	s.commands.Lock()
	return err
}

func (s *Session) Jobs() ([]Job, error) { return s.manager.Status(context.Background()) }
