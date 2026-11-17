package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadOptions configures one sequential transfer. No resume state is stored.
type DownloadOptions struct {
	URL         string
	Destination string
	SHA256      string
	Progress    func(written, total int64)
}

// Download writes a private, job-owned partial beside Destination. On failure it
// removes that partial. Success requires verification, sync, close, and exclusive
// publication by hard link (never replacing an existing destination or symlink).
// This is process-level finalization, not a crash-durable M2 checkpoint protocol.
func Download(ctx context.Context, client *http.Client, options DownloadOptions) (err error) {
	if options.Destination == "" {
		return errors.New("destination is required")
	}
	expected, err := hex.DecodeString(options.SHA256)
	if err != nil || (options.SHA256 != "" && len(expected) != sha256.Size) {
		return errors.New("SHA-256 must contain 64 hexadecimal characters")
	}
	if _, err := os.Lstat(options.Destination); err == nil {
		return errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(options.Destination), "."+filepath.Base(options.Destination)+"-*.part")
	if err != nil {
		return fmt.Errorf("create partial: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, file.Close())
		}
		if removeErr := os.Remove(file.Name()); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove partial: %w", removeErr))
		}
	}()
	hash := sha256.New()
	if err := Transfer(ctx, client, options.URL, io.MultiWriter(file, hash), options.Progress); err != nil {
		return err
	}
	if options.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), options.SHA256) {
		return errors.New("SHA-256 mismatch")
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync partial: %w", err)
	}
	closed = true
	if err := file.Close(); err != nil {
		return fmt.Errorf("close partial: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(file.Name(), options.Destination); err != nil {
		return fmt.Errorf("publish destination: %w", err)
	}
	return nil
}
