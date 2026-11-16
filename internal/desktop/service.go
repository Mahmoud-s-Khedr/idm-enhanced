// Package desktop presents the platform spike's connection state.
package desktop

import (
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"runtime"
	"sync"
)

type Status struct {
	Build    string `json:"build"`
	Platform string `json:"platform"`
	LastPing string `json:"lastPing"`
	IPC      string `json:"ipc"`
}
type Service struct {
	mu   sync.RWMutex
	last string
	ipc  string
}

func New(ipcStatus string) *Service { return &Service{ipc: ipcStatus} }
func (s *Service) Info() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Build: "M0 / Wails 3.0.0", Platform: runtime.GOOS + "/" + runtime.GOARCH, LastPing: s.last, IPC: s.ipc}
}
func (s *Service) Hello() string { return "Connected to the Go desktop service." }

//wails:ignore
func (s *Service) Accept(m protocol.Message) (protocol.Message, error) {
	reply, e := protocol.Ack(m)
	if e != nil {
		return reply, e
	}
	s.mu.Lock()
	s.last = m.ID
	s.mu.Unlock()
	return reply, nil
}
