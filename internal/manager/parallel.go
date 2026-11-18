package manager

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
)

var errSegment = errors.New("switch to segmented transfer")

const rangeChunk = int64(4 * 1024 * 1024)

func (m *Manager) stream(ctx context.Context, j *Job, identity engine.Representation, resumed bool, w *checkpointWriter, progress func(int64, int64), inspect func(engine.Representation) error) (engine.Representation, error) {
	parallel := m.config.Workers > 1 && resumed && j.Length-j.Durable > rangeChunk
	var rep engine.Representation
	var err error
	if !parallel {
		rep, err = engine.Stream(ctx, m.client, engine.Request{Source: j.Source, Resume: resumed, Offset: j.Durable, Identity: identity}, w, progress, func(r engine.Representation) error {
			if err := inspect(r); err != nil {
				return err
			}
			if !resumed && m.config.Workers > 1 && r.Length > rangeChunk && engine.StrongETag(r.ETag) {
				return errSegment
			}
			return nil
		})
		if !segmentSwitch(err) {
			return rep, err
		}
		identity = rep
		_, err = engine.Stream(ctx, m.client, engine.Request{Source: j.Source, Resume: true, Probe: true, Identity: identity}, io.Discard, nil, nil)
		if errors.Is(err, engine.ErrUnsafeResume) {
			// No bytes have been accepted. Servers ignoring the initial range probe can
			// still be downloaded safely using a fresh complete response.
			return engine.Stream(ctx, m.client, engine.Request{Source: j.Source}, w, progress, inspect)
		}
		if err != nil {
			return rep, err
		}
	}
	for w.position < identity.Length {
		end, transferErr := engine.Parallel(ctx, m.client, j.Source, identity, w.file, w.position, rangeChunk, m.config.Workers)
		w.position = end
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// All workers have joined. Sync before persisting only the successful prefix.
		err = w.checkpoint(cleanup)
		cancel()
		if err != nil {
			return identity, errors.Join(transferErr, err)
		}
		if transferErr != nil {
			return identity, transferErr
		}
		if progress != nil {
			progress(w.position, identity.Length)
		}
	}
	// WriteAt does not move the file offset used by the sequential checkpoint path.
	_, err = w.file.Seek(w.position, io.SeekStart)
	return identity, err
}

// A response-close failure must not be swallowed by the inspection handoff.
func segmentSwitch(err error) bool {
	if err == errSegment {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		return len(causes) == 1 && segmentSwitch(causes[0])
	}
	return false
}
