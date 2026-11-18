package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
)

func desktopFixture(t *testing.T) *Service {
	t.Helper()
	m, err := manager.Open(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.NewSession(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	s := New("ready")
	s.Attach(session, PlatformActions{})
	return s
}
func desktopWait(t *testing.T, s *Service) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		snapshot, err := s.Downloads()
		if err != nil {
			t.Fatal(err)
		}
		if !snapshot.Running {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatal("desktop run did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestDesktopDownloadWorkflow(t *testing.T) {
	s := desktopFixture(t)
	var opened string
	var revealed, quit bool
	s.actions = PlatformActions{ChooseDestination: func() (string, error) { return "/selected/file", nil }, Open: func(path string, reveal bool) error { opened = path; revealed = reveal; return nil }, Quit: func() { quit = true }}
	if path, err := s.ChooseDestination(); err != nil || path != "/selected/file" {
		t.Fatal(path, err)
	}
	s.actions.AuthorizeNotifications = func() error { return nil }
	if err := s.Control(Command{Action: "authorize-notifications"}); err != nil {
		t.Fatal(err)
	}
	s.actions.AuthorizeNotifications = nil
	if s.Control(Command{Action: "authorize-notifications"}) == nil {
		t.Fatal("missing notification permission adapter")
	}
	s.Quit()
	if !quit {
		t.Fatal("quit not forwarded")
	}
	s.NotificationFailure()
	if s.Info().Notification == "" {
		t.Fatal("notification failure missing")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("desktop bytes")) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	id, err := s.Add(DownloadRequest{URL: server.URL + "/file?secret=hidden", Destination: path, Scheduled: "2030-01-01T12:00:00+02:00"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.Add(DownloadRequest{URL: server.URL, Destination: filepath.Join(t.TempDir(), "second")})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Downloads()
	if err != nil || len(snapshot.Jobs) != 2 || snapshot.Jobs[0].Scheduled != "2030-01-01T10:00:00Z" {
		t.Fatal(snapshot, err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil || strings.Contains(string(data), "secret") || strings.Contains(string(data), "hidden") {
		t.Fatal("unsafe snapshot", string(data), err)
	}
	if s.Control(Command{Action: "open", ID: id}) == nil {
		t.Fatal("opened incomplete file")
	}
	if err = s.Control(Command{Action: "reorder", Order: []string{id2, id}}); err != nil {
		t.Fatal(err)
	}
	if err = s.Control(Command{Action: "schedule", ID: id, At: ""}); err != nil {
		t.Fatal(err)
	}
	p := snapshot.Settings
	p.Workers = 2
	if err = s.SaveSettings(p); err != nil {
		t.Fatal(err)
	}
	if err = s.Control(Command{Action: "run-queue"}); err != nil {
		t.Fatal(err)
	}
	snapshot = desktopWait(t, s)
	for _, j := range snapshot.Jobs {
		if j.State != "completed" {
			t.Fatal(j)
		}
	}
	for _, action := range []string{"open", "reveal"} {
		if err = s.Control(Command{Action: action, ID: id}); err != nil {
			t.Fatal(err)
		}
		if opened != path || revealed != (action == "reveal") {
			t.Fatal(opened, revealed)
		}
	}
	if err = s.Control(Command{Action: "remove", ID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("history removed output", err)
	}
	if err = s.Control(Command{Action: "pause-all"}); err != nil {
		t.Fatal(err)
	}
	if s.Control(Command{Action: "resume", ID: "missing"}) == nil || s.Control(Command{Action: "restart", ID: "missing"}) == nil {
		t.Fatal("missing job accepted")
	}
	if s.Control(Command{Action: "open", ID: "missing"}) == nil {
		t.Fatal("missing file opened")
	}
	s.actions.Open = nil
	if s.Control(Command{Action: "open", ID: id2}) == nil {
		t.Fatal("missing platform action accepted")
	}
}
func TestDesktopBoundaryValidation(t *testing.T) {
	empty := New("ready")
	empty.Quit()
	if _, err := empty.Downloads(); err == nil {
		t.Fatal("no session")
	}
	if _, err := empty.Add(DownloadRequest{}); err == nil {
		t.Fatal("no session")
	}
	if empty.Control(Command{}) == nil || empty.SaveSettings(manager.Settings{}) == nil {
		t.Fatal("no session")
	}
	if _, err := empty.ChooseDestination(); err == nil {
		t.Fatal("missing chooser")
	}
	s := desktopFixture(t)
	for _, r := range []DownloadRequest{{URL: strings.Repeat("x", 16385)}, {Destination: strings.Repeat("x", 4097)}, {SHA256: strings.Repeat("a", 65)}, {Scheduled: "2030-01-01T12:00"}, {URL: "file:///etc/passwd", Destination: filepath.Join(t.TempDir(), "file")}} {
		if _, err := s.Add(r); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	for _, c := range []Command{{Action: "resume"}, {ID: strings.Repeat("a", 65)}, {Order: make([]string, 10001)}, {Action: "schedule", At: "invalid"}, {Action: "bogus"}} {
		if s.Control(c) == nil {
			t.Fatal("invalid command accepted")
		}
	}
	if _, err := s.Add(DownloadRequest{URL: "https://example.invalid/file", Destination: filepath.Join(t.TempDir(), "file")}); err != nil {
		t.Fatal(err)
	}
	if err := s.session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Downloads(); err == nil {
		t.Fatal("closed backend readable")
	}
	if s.Control(Command{Action: "open", ID: "missing"}) == nil {
		t.Fatal("closed backend opened file")
	}
}
