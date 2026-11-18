// Package desktop presents the platform spike's connection state.
package desktop

import (
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Status struct {
	Build        string `json:"build"`
	Platform     string `json:"platform"`
	LastPing     string `json:"lastPing"`
	IPC          string `json:"ipc"`
	Notification string `json:"notification"`
	Lifecycle    string `json:"lifecycle"`
}
type Service struct {
	mu           sync.RWMutex
	last         string
	ipc          string
	session      *manager.Session
	actions      PlatformActions
	notification string
	lifecycle    string
}

func New(ipcStatus string) *Service { return &Service{ipc: ipcStatus} }
func (s *Service) Info() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Build: "M7 / Wails 3.0.0", Platform: runtime.GOOS + "/" + runtime.GOARCH, LastPing: s.last, IPC: s.ipc, Notification: s.notification, Lifecycle: s.lifecycle}
}
func (s *Service) Hello() string { return "Connected to the Go desktop service." }

//wails:ignore
func (s *Service) Accept(m protocol.Message) (protocol.Message, error) {
	if err := protocol.ValidateRequest(m); err != nil {
		return protocol.Message{}, err
	}
	if m.Type == "download" {
		reply := protocol.Message{Version: protocol.Version, ID: m.ID, Type: "error", Error: "Download was not accepted. Open the desktop and check the download folder or request ID; retry the same request."}
		if s.session == nil {
			return reply, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return reply, nil
		}
		directory := filepath.Join(home, "Downloads")
		if err = os.MkdirAll(directory, 0755); err != nil {
			return reply, nil
		}
		id, err := s.session.AcceptBrowser(m, directory)
		if err != nil {
			return reply, nil
		}
		return protocol.Message{Version: protocol.Version, ID: m.ID, Type: "accepted", JobID: id}, nil
	}
	reply, err := protocol.Ack(m)
	if err != nil {
		return reply, err
	}
	s.mu.Lock()
	s.last = m.ID
	s.mu.Unlock()
	return reply, nil
}

// Disconnect presents bridge failure without exposing transport details.
//
//wails:ignore
func (s *Service) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipc = "Browser bridge disconnected. Restart the application."
}

//wails:ignore
func (s *Service) NotificationFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notification = "System notifications unavailable. Completed downloads remain in History."
}

//wails:ignore
func (s *Service) SetTrayAvailable(available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if available {
		s.lifecycle = "Closing this window keeps downloads running in the system tray."
	} else {
		s.lifecycle = "No system tray host is available. Closing this window minimizes it to the taskbar; downloads keep running."
	}
}

//wails:ignore
func (s *Service) NotificationReady() { s.mu.Lock(); defer s.mu.Unlock(); s.notification = "" }
