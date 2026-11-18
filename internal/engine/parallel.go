package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
)

// Parallel writes a bounded window of disjoint ranges and joins every worker
// before reporting its contiguous successful prefix. Callers sync and checkpoint
// that prefix; speculative bytes beyond it must never count as durable progress.
func Parallel(ctx context.Context, client *http.Client, source string, identity Representation, dst io.WriterAt, start, chunk int64, workers int) (int64, error) {
	if client == nil || dst == nil || workers < 1 || workers > 16 || (chunk < 1 || chunk > 64*1024*1024) || start < 0 || start > identity.Length {
		return start, errors.New("invalid parallel limits")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ends := make([]int64, workers)
	failures := make([]error, workers)
	var wg sync.WaitGroup
	next := start
	for i := range workers {
		offset := next
		if offset >= identity.Length {
			break
		}
		end := offset + min(chunk, identity.Length-offset)
		next = end
		wg.Go(func() {
			w := &offsetWriter{dst: dst, offset: offset}
			_, err := Stream(ctx, client, Request{Source: source, Resume: true, Offset: offset, End: end, Identity: identity}, w, nil, nil)
			if err != nil {
				failures[i] = err
				cancel()
				return
			}
			ends[i] = end
		})
	}
	wg.Wait()
	prefix := start
	for _, end := range ends {
		if end == 0 {
			break
		}
		prefix = end
	}
	// Prefer the originating error over cancellation of sibling requests.
	var failure error
	for _, err := range failures {
		if err != nil && !errors.Is(err, context.Canceled) {
			failure = errors.Join(failure, err)
		}
	}
	if failure == nil {
		for _, err := range failures {
			failure = errors.Join(failure, err)
		}
	}
	return prefix, failure
}

type offsetWriter struct {
	dst    io.WriterAt
	offset int64
}

func (w *offsetWriter) Write(p []byte) (int, error) {
	n, err := w.dst.WriteAt(p, w.offset)
	w.offset += int64(n)
	return n, err
}
