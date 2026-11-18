package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func sessionFixture(t *testing.T, notify func(Job)) (*Session, string) {
	t.Helper()
	dir := t.TempDir()
	m, err := Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(m, notify)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, dir
}
func sessionJob(t *testing.T, s *Session, source string) Job {
	t.Helper()
	j, err := s.Create(Options{URL: source, Destination: filepath.Join(t.TempDir(), "download")})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func sessionWait(t *testing.T, s *Session) {
	t.Helper()
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session did not join")
	}
}
func TestSessionQueueHistoryAndSettings(t *testing.T) {
	var notices atomic.Int32
	s, dir := sessionFixture(t, func(Job) { notices.Add(1) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("verified output")) }))
	defer server.Close()
	a := sessionJob(t, s, server.URL)
	if _, err := s.Create(Options{URL: server.URL, Destination: a.Destination}); err == nil {
		t.Fatal("duplicate unfinished destination accepted")
	}
	b := sessionJob(t, s, server.URL)
	p := s.Settings()
	p.Workers = 2
	p.Jobs = 1
	if err := s.Configure(p); err != nil {
		t.Fatal(err)
	}
	bad := p
	bad.Jobs = 0
	if s.Configure(bad) == nil {
		t.Fatal("invalid limit")
	}
	if s.Start("missing", false) == nil {
		t.Fatal("missing job admitted")
	}
	if s.Command("reorder", "", false, time.Time{}, []string{b.ID, a.ID}) != nil {
		t.Fatal("reorder")
	}
	if s.Command("bogus", a.ID, false, time.Time{}, nil) == nil {
		t.Fatal("unknown command")
	}
	if s.Command("remove", a.ID, false, time.Time{}, nil) == nil {
		t.Fatal("unfinished history removed")
	}
	if err := s.Start("", false); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	jobs, err := s.Jobs()
	if err != nil || len(jobs) != 2 || notices.Load() != 2 {
		t.Fatal(jobs, err, notices.Load())
	}
	for _, j := range jobs {
		if j.State != "completed" {
			t.Fatal(j)
		}
		data, err := os.ReadFile(j.Destination)
		if err != nil || string(data) != "verified output" {
			t.Fatal(err, string(data))
		}
	}
	if err = s.Command("remove", a.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(a.Destination); err != nil {
		t.Fatal("completed file removed", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(Options{}); err == nil {
		t.Fatal("create after close")
	}
	if s.Start(b.ID, false) == nil || s.Configure(p) == nil || s.Command("cancel", b.ID, true, time.Time{}, nil) == nil {
		t.Fatal("closed session accepted commands")
	}
	m, err := Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewSession(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	if again.Settings() != p {
		t.Fatal("settings not persisted", again.Settings())
	}
}
func TestSessionScheduleSleepWakeAndPauseAll(t *testing.T) {
	s, _ := sessionFixture(t, nil)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write([]byte("scheduled")) }))
	defer server.Close()
	j := sessionJob(t, s, server.URL)
	future := time.Now().Add(time.Hour)
	if err := s.Command("schedule", j.ID, false, future, nil); err != nil {
		t.Fatal(err)
	}
	s.tick(time.Now())
	if requests.Load() != 0 {
		t.Fatal("early schedule")
	}
	if err := s.Start("", false); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	if requests.Load() != 0 {
		t.Fatal("queue bypassed schedule")
	}
	s.Sleep()
	s.tick(future.Add(time.Second))
	if s.Start(j.ID, false) == nil {
		t.Fatal("start while asleep")
	}
	_, sleeping, _ := s.State()
	if !sleeping {
		t.Fatal("sleep state missing")
	}
	s.Wake()
	s.PauseAll()
	s.tick(future.Add(time.Second))
	if requests.Load() != 0 {
		t.Fatal("pause all allowed scheduled admission")
	}
	// Explicit run resumes schedule admission, but still respects future time.
	if err := s.Start("", false); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	s.tick(future.Add(time.Second))
	sessionWait(t, s)
	jobs, err := s.Jobs()
	if err != nil || jobs[0].State != "completed" || requests.Load() == 0 {
		t.Fatal(jobs, err)
	}
	if s.Command("schedule", j.ID, false, time.Time{}, nil) == nil {
		t.Fatal("scheduled completed job")
	}
	if s.Command("pause", j.ID, false, time.Time{}, nil) == nil {
		t.Fatal("paused completed job")
	}
}
func TestSessionJoinsBeforeCloseAndRejectsMutation(t *testing.T) {
	for _, operation := range []string{"sleep", "pause-all", "close", "pause"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := sessionFixture(t, nil)
			started := make(chan struct{}, 1)
			released := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-r.Context().Done()
				released <- struct{}{}
			}))
			defer server.Close()
			j := sessionJob(t, s, server.URL)
			if err := s.Start(j.ID, false); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request not started")
			}
			if s.Start(j.ID, false) == nil || s.Configure(s.Settings()) == nil {
				t.Fatal("busy run accepted")
			}
			for _, action := range []string{"cancel", "remove", "schedule", "reorder"} {
				if s.Command(action, j.ID, false, time.Time{}, nil) == nil {
					t.Fatal("active mutation", action)
				}
			}
			if s.manager.Schedule(context.Background(), j.ID, time.Now()) == nil || s.manager.PauseQueued(context.Background(), j.ID) == nil || s.manager.Remove(context.Background(), j.ID) == nil {
				t.Fatal("manager active mutation")
			}
			switch operation {
			case "sleep":
				s.Sleep()
				s.Wake()
			case "pause-all":
				s.PauseAll()
			case "close":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			case "pause":
				if err := s.Command("pause", j.ID, false, time.Time{}, nil); err != nil {
					t.Fatal(err)
				}
				sessionWait(t, s)
			}
			select {
			case <-released:
			case <-time.After(5 * time.Second):
				t.Fatal("request leaked")
			}
			if operation != "close" {
				jobs, err := s.Jobs()
				if err != nil || jobs[0].State != "paused" {
					t.Fatal(jobs, err)
				}
			}
		})
	}
}
func TestSessionCancellationAndFailures(t *testing.T) {
	s, _ := sessionFixture(t, nil)
	j := sessionJob(t, s, "http://127.0.0.1:1/secret?token=never-log")
	if err := s.Command("pause", j.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Command("cancel", j.ID, true, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Command("remove", j.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if s.Command("pause", "missing", false, time.Time{}, nil) == nil || s.Command("remove", "missing", false, time.Time{}, nil) == nil || s.Command("schedule", "missing", false, time.Time{}, nil) == nil {
		t.Fatal("missing job accepted")
	}
	failure := sessionJob(t, s, "http://127.0.0.1:1/secret?token=never-log")
	p := s.Settings()
	p.Retries = 0
	p.Notifications = false
	if err := s.Configure(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(failure.ID, false); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	_, _, message := s.State()
	if message == "" {
		t.Fatal("failure invisible")
	}
	if err := s.Start(failure.ID, true); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	if err := s.Command("cancel", failure.ID, true, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if s.Command("remove", failure.ID, false, time.Time{}, nil) == nil {
		t.Fatal("retained partial orphaned")
	}
	if err := s.Command("cancel", failure.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Command("remove", failure.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
}
func TestSessionInvalidSavedSettings(t *testing.T) {
	for _, data := range []string{`{"jobs":"invalid"}`, `{"jobs":0}`} {
		m, err := Open(t.TempDir(), false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = m.store.SavePreferences(context.Background(), []byte(data)); err != nil {
			t.Fatal(err)
		}
		if _, err = NewSession(m, nil); err == nil {
			t.Fatal("invalid saved settings accepted")
		}
		if err = m.Close(); err != nil {
			t.Fatal(err)
		}
	}
	m, err := Open(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = NewSession(m, nil); err == nil {
		t.Fatal("closed store accepted")
	}
}
func TestSessionScheduleReopen(t *testing.T) {
	s, dir := sessionFixture(t, nil)
	j := sessionJob(t, s, "https://example.invalid/file")
	at := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := s.Command("schedule", j.ID, false, at, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := Open(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	jobs, err := m.Status(context.Background())
	if err != nil || !jobs[0].ScheduledAt.Equal(at) {
		t.Fatal(jobs, err)
	}
	if err = m.Schedule(context.Background(), j.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionAdmissionReservation(t *testing.T) {
	s, _ := sessionFixture(t, nil)
	j := sessionJob(t, s, "https://example.invalid/file")
	// Hold actual manager admission so this checks the reservation window rather
	// than merely re-testing active-worker mutation rejection.
	func() {
		s.manager.mu.Lock()
		defer s.manager.mu.Unlock()
		if err := s.Start(j.ID, false); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"cancel", "remove", "schedule", "reorder"} {
			if s.Command(action, j.ID, false, time.Time{}, nil) == nil {
				t.Fatal("mutation bypassed run reservation", action)
			}
		}
	}()
	s.PauseAll()
}
