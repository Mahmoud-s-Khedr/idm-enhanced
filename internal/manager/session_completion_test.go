package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionPauseReservedRun(t *testing.T) {
	s, _ := sessionFixture(t, nil)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write([]byte("output")) }))
	defer server.Close()
	j := sessionJob(t, s, server.URL)
	reserved := make(chan struct{})
	s.beforeRun = func(ctx context.Context) { close(reserved); <-ctx.Done() }
	if err := s.Start(j.ID, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reserved:
	case <-time.After(5 * time.Second):
		t.Fatal("run not reserved")
	}
	if err := s.Command("pause", j.ID, false, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	running, _, _ := s.State()
	saved, err := s.manager.store.Get(context.Background(), j.ID)
	if err != nil || running || saved.State != "paused" || requests.Load() != 0 {
		t.Fatal(saved, running, requests.Load(), err)
	}
}

func TestSessionNotifiesBeforeQueueDrains(t *testing.T) {
	notices := make(chan Job, 3)
	s, _ := sessionFixture(t, func(j Job) { notices <- j })
	stalled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			select {
			case stalled <- struct{}{}:
			default:
			}
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("verified output"))
	}))
	defer server.Close()
	defer s.PauseAll()
	small := sessionJob(t, s, server.URL+"/small")
	sessionJob(t, s, server.URL+"/stall")
	if err := s.Start("", false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stalled:
	case <-time.After(5 * time.Second):
		t.Fatal("second job did not start")
	}
	select {
	case j := <-notices:
		if j.ID != small.ID || j.State != "completed" {
			t.Fatal(j)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completed job notification waited for stalled peer")
	}
	if running, _, _ := s.State(); !running {
		t.Fatal("queue unexpectedly drained")
	}
	s.PauseAll()
	select {
	case j := <-notices:
		t.Fatal("interrupted job notified", j)
	default:
	}
	// Reopening an already completed job must not produce a duplicate notice.
	if err := s.Start(small.ID, false); err != nil {
		t.Fatal(err)
	}
	sessionWait(t, s)
	select {
	case j := <-notices:
		t.Fatal("duplicate completion notice", j)
	default:
	}
}
