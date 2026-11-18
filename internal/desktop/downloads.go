package desktop

import (
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
)

// JobView intentionally excludes source URLs, validators and ownership tokens.
type JobView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Host        string `json:"host"`
	State       string `json:"state"`
	Error       string `json:"error"`
	Guidance    string `json:"guidance"`
	Bytes       int64  `json:"bytes"`
	Total       int64  `json:"total"`
	Order       int64  `json:"order"`
	Scheduled   string `json:"scheduled"`
	RetryAt     string `json:"retryAt"`
}
type Snapshot struct {
	Jobs     []JobView        `json:"jobs"`
	Running  bool             `json:"running"`
	Sleeping bool             `json:"sleeping"`
	Message  string           `json:"message"`
	Settings manager.Settings `json:"settings"`
}
type DownloadRequest struct {
	URL         string `json:"url"`
	Destination string `json:"destination"`
	SHA256      string `json:"sha256"`
	Scheduled   string `json:"scheduled"`
}
type Command struct {
	Action      string   `json:"action"`
	ID          string   `json:"id"`
	KeepPartial bool     `json:"keepPartial"`
	At          string   `json:"at"`
	Order       []string `json:"order"`
}

// PlatformActions is the narrow OS boundary; paths are resolved from saved jobs.
type PlatformActions struct {
	ChooseDestination      func() (string, error)
	Open                   func(string, bool) error
	AuthorizeNotifications func() error
	Quit                   func()
}

//wails:ignore
func (s *Service) Attach(session *manager.Session, actions PlatformActions) {
	s.session = session
	s.actions = actions
}
func (s *Service) Downloads() (Snapshot, error) {
	if s.session == nil {
		return Snapshot{}, errors.New("downloads unavailable")
	}
	jobs, err := s.session.Jobs()
	if err != nil {
		return Snapshot{}, errors.New("unable to read downloads")
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].QueueOrder != jobs[j].QueueOrder {
			return jobs[i].QueueOrder < jobs[j].QueueOrder
		}
		return jobs[i].Created.Before(jobs[j].Created)
	})
	out := Snapshot{Jobs: make([]JobView, 0, len(jobs)), Settings: s.session.Settings()}
	out.Running, out.Sleeping, out.Message = s.session.State()
	for _, j := range jobs {
		host := ""
		if u, e := url.Parse(j.Source); e == nil {
			host = u.Hostname()
		}
		out.Jobs = append(out.Jobs, JobView{ID: j.ID, Name: filepath.Base(j.Destination), Destination: j.Destination, Host: host, State: j.State, Error: manager.SafeError(j), Guidance: manager.RecoveryHint(j), Bytes: j.Durable, Total: j.Length, Order: j.QueueOrder, Scheduled: instant(j.ScheduledAt), RetryAt: instant(j.RetryAt)})
	}
	return out, nil
}
func instant(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}
func parseInstant(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("schedule must include a timezone")
	}
	return at.UTC(), nil
}
func (s *Service) Add(r DownloadRequest) (string, error) {
	if s.session == nil {
		return "", errors.New("downloads unavailable")
	}
	if len(r.URL) > 16384 || len(r.Destination) > 4096 || len(r.SHA256) > 64 {
		return "", errors.New("download input is too long")
	}
	at, err := parseInstant(r.Scheduled)
	if err != nil {
		return "", err
	}
	j, err := s.session.Create(manager.Options{URL: r.URL, Destination: r.Destination, SHA256: r.SHA256, ScheduledAt: at})
	return j.ID, err
}
func (s *Service) Control(c Command) error {
	if s.session == nil {
		return errors.New("downloads unavailable")
	}
	if len(c.ID) > 64 || len(c.Order) > 10000 {
		return errors.New("invalid command size")
	}
	switch c.Action {
	case "resume", "restart", "pause", "cancel", "remove", "schedule", "open", "reveal":
		if c.ID == "" {
			return errors.New("job ID is required")
		}
	}
	switch c.Action {
	case "authorize-notifications":
		if s.actions.AuthorizeNotifications == nil {
			return errors.New("notification authorization unavailable")
		}
		return s.actions.AuthorizeNotifications()
	case "run-queue":
		return s.session.Start("", false)
	case "resume", "restart":
		return s.session.Start(c.ID, c.Action == "restart")
	case "pause-all":
		s.session.PauseAll()
		return nil
	case "open", "reveal":
		jobs, err := s.session.Jobs()
		if err != nil {
			return errors.New("unable to read downloads")
		}
		for _, j := range jobs {
			if j.ID == c.ID {
				if j.State != "completed" {
					return errors.New("file is not complete")
				}
				if s.actions.Open == nil {
					return errors.New("file actions unavailable")
				}
				return s.actions.Open(j.Destination, c.Action == "reveal")
			}
		}
		return errors.New("job not found")
	default:
		at, err := parseInstant(c.At)
		if err != nil {
			return err
		}
		return s.session.Command(c.Action, c.ID, c.KeepPartial, at, c.Order)
	}
}
func (s *Service) SaveSettings(p manager.Settings) error {
	if s.session == nil {
		return errors.New("downloads unavailable")
	}
	return s.session.Configure(p)
}
func (s *Service) ChooseDestination() (string, error) {
	if s.actions.ChooseDestination == nil {
		return "", errors.New("destination chooser unavailable")
	}
	return s.actions.ChooseDestination()
}
func (s *Service) Quit() {
	if s.actions.Quit != nil {
		s.actions.Quit()
	}
}
