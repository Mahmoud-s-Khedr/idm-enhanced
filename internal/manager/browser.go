package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

// AcceptBrowser serializes durable deduplication and destination reservation with
// all other manager admissions. Receipts survive metadata-only history removal.
func (m *Manager) AcceptBrowser(ctx context.Context, request protocol.Message, directory string) (string, error) {
	if err := protocol.ValidateRequest(request); err != nil {
		return "", err
	}
	if request.Type != "download" {
		return "", errors.New("expected download")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	input := request.URL + "\x00" + request.Filename
	// Preserve M5 file-receipt fingerprints exactly; bind the additive HLS mode
	// so the same request ID cannot silently change job semantics.
	if request.Mode != "" {
		input += "\x00" + request.Mode
	}
	digest := sha256.Sum256([]byte(input))
	fingerprint := hex.EncodeToString(digest[:])
	old, jobID, err := m.store.BrowserRequest(ctx, request.ID)
	if err != nil {
		return "", err
	}
	if jobID != "" {
		if old != fingerprint {
			return "", errors.New("request ID already used for another download")
		}
		return jobID, nil
	}
	jobs, err := m.store.List(ctx)
	if err != nil {
		return "", err
	}
	// The directory comes from the desktop, never the browser. Check ownership and
	// symlinks through the existing allocation path when the job is actually run.
	if !filepath.IsAbs(directory) {
		return "", errors.New("download directory must be absolute")
	}
	for n := 0; n < 100; n++ {
		name := request.Filename
		if request.Mode == "hls" {
			name = strings.TrimSuffix(name, filepath.Ext(name))
			if name == "" || len(name) > 176 {
				name = "media"
			}
			name += ".mp4"
		}
		if n > 0 {
			ext := filepath.Ext(name)
			name = fmt.Sprintf("%s (%d)%s", name[:len(name)-len(ext)], n, ext)
		}
		dest := filepath.Join(directory, name)
		occupied := false
		for _, j := range jobs {
			if sameDestination(j.Destination, dest) && j.State != "cancelled" && j.State != "completed" {
				occupied = true
				break
			}
		}
		if occupied {
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		j, err := m.create(ctx, Options{Kind: request.Mode, URL: request.URL, Destination: dest}, request.ID, fingerprint)
		return j.ID, err
	}
	return "", errors.New("choose another filename; too many destination collisions")
}
func (s *Session) AcceptBrowser(request protocol.Message, directory string) (string, error) {
	s.commands.Lock()
	defer s.commands.Unlock()
	if err := s.available(); err != nil {
		return "", err
	}
	return s.manager.AcceptBrowser(context.Background(), request, directory)
}
