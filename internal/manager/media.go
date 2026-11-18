package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/localfs"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/media"
)

var errMediaOwnership = errors.New("media cache ownership or data changed; explicit restart required")

type mediaObject struct {
	File, Identity, Ownership, Digest string
	Remote                            engine.Representation
	Length                            int64
	Complete                          bool
}
type mediaCheckpoint struct {
	Version                      int
	Plan                         media.Plan
	Directory, DirectoryIdentity string
	Objects                      []mediaObject
}

func decodeMedia(j Job) (mediaCheckpoint, error) {
	var cp mediaCheckpoint
	if len(j.Media) == 0 || len(j.Media) > 4<<20 || json.Unmarshal(j.Media, &cp) != nil || cp.Version != 1 || len(cp.Objects) == 0 || len(cp.Objects) != len(cp.Plan.Segments) || len(cp.Objects) > media.MaxObjects {
		return cp, errMediaOwnership
	}
	for _, r := range cp.Objects {
		if r.File != "" && (filepath.Dir(r.File) != cp.Directory || r.Identity == "" || r.Ownership == "") {
			return cp, errMediaOwnership
		}
		if r.Complete && (r.File == "" || r.Length <= 0 || len(r.Digest) != 64) {
			return cp, errMediaOwnership
		}
	}
	return cp, nil
}
func (m *Manager) saveMedia(ctx context.Context, j *Job, cp mediaCheckpoint) error {
	b, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return media.ErrLimit
	}
	next := *j
	next.Media = b
	next.MediaLoaded = true
	next.MediaObjects = len(cp.Objects)
	next.MediaComplete = 0
	next.MediaBytes = 0
	for _, r := range cp.Objects {
		if r.Complete {
			next.MediaComplete++
			next.MediaBytes += r.Length
		}
	}
	next.Generation++
	if err = m.store.Save(ctx, next); err != nil {
		return err
	}
	*j = next
	return nil
}
func (m *Manager) checkMediaDirectory(j Job, cp mediaCheckpoint) error {
	if filepath.Dir(cp.Directory) != m.directory || !strings.HasPrefix(filepath.Base(cp.Directory), "media-"+j.ID+"-") {
		return errMediaOwnership
	}
	if err := localfs.CheckProtected(cp.Directory, true); err != nil {
		return errMediaOwnership
	}
	f, err := os.Open(cp.Directory)
	if err != nil {
		return errMediaOwnership
	}
	identity, e := localfs.Identity(f)
	err = errors.Join(e, f.Close())
	if err != nil || identity != cp.DirectoryIdentity {
		return errMediaOwnership
	}
	return nil
}
func (m *Manager) createMedia(ctx context.Context, j *Job) (mediaCheckpoint, error) {
	plan, err := media.Resolve(ctx, m.client, j.Source, j.MediaChoice)
	if err != nil {
		return mediaCheckpoint{}, err
	}
	dir, err := os.MkdirTemp(m.directory, "media-"+j.ID+"-")
	if err != nil {
		return mediaCheckpoint{}, err
	}
	cp := mediaCheckpoint{Version: 1, Plan: plan, Directory: dir, Objects: make([]mediaObject, len(plan.Segments))}
	if err = localfs.Protect(dir, true); err != nil {
		return cp, err
	}
	f, err := os.Open(dir)
	if err != nil {
		return cp, err
	}
	cp.DirectoryIdentity, err = localfs.Identity(f)
	err = errors.Join(err, f.Close())
	if err != nil {
		return cp, err
	}
	if err = localfs.SyncDir(m.directory); err != nil {
		return cp, err
	}
	return cp, m.saveMedia(ctx, j, cp)
}
func allMediaComplete(cp mediaCheckpoint) bool {
	for _, r := range cp.Objects {
		if !r.Complete {
			return false
		}
	}
	return len(cp.Objects) > 0
}
func (m *Manager) inspectMediaResume(ctx context.Context, j *Job, cp *mediaCheckpoint) error {
	plan, err := media.Resolve(ctx, m.client, j.Source, j.MediaChoice)
	if err != nil {
		return err
	}
	if plan.SourceDigest != cp.Plan.SourceDigest || plan.PlaylistDigest != cp.Plan.PlaylistDigest || plan.PlaylistURL != cp.Plan.PlaylistURL || len(plan.Segments) != len(cp.Plan.Segments) {
		return engine.ErrUnsafeResume
	}
	for i, uri := range plan.Segments {
		if uri != cp.Plan.Segments[i] {
			return engine.ErrUnsafeResume
		}
	}
	changed := false
	for i, r := range cp.Objects {
		if !r.Complete {
			continue
		}
		if err := checkMediaObject(ctx, r); err != nil {
			return err
		}
		if !engine.StrongETag(r.Remote.ETag) {
			cp.Objects[i].Complete = false
			changed = true
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = engine.Stream(probeCtx, m.client, engine.Request{Source: cp.Plan.Segments[i], Resume: true, Probe: true, Identity: r.Remote}, io.Discard, nil, nil)
		cancel()
		if err != nil {
			return err
		}
	}
	if changed {
		return m.saveMedia(ctx, j, *cp)
	}
	return nil
}
func checkMediaObject(ctx context.Context, r mediaObject) error {
	f, err := openOwnedFile(r.File, r.Identity, r.Ownership, os.O_RDONLY)
	if err != nil {
		return errMediaOwnership
	}
	info, e := f.Stat()
	sum, se := digest(contextReader{ctx: ctx, reader: f})
	err = errors.Join(e, se, f.Close())
	if err != nil {
		return err
	}
	if info.Size() != r.Length || sum != r.Digest {
		return errMediaOwnership
	}
	return nil
}

func (m *Manager) runMedia(ctx context.Context, j *Job) (err error) {
	defer func() {
		if err != nil && j.Phase != "verified" && (j.State == "analyzing" || j.State == "downloading" || j.State == "assembling" || j.State == "verifying") {
			err = m.mediaFailure(j, err)
		}
	}()
	j.Error = ""
	if err := m.transition(ctx, j, "analyzing"); err != nil {
		return err
	}
	fresh := len(j.Media) == 0
	cp, err := decodeMedia(*j)
	if fresh {
		cp, err = m.createMedia(ctx, j)
	}
	if err != nil {
		return m.mediaFailure(j, err)
	}
	if err = m.checkMediaDirectory(*j, cp); err != nil {
		return m.mediaFailure(j, err)
	}
	// Once every object is durably complete, assembly retries are entirely local,
	// including after signed URLs expire or the server becomes unavailable.
	if !allMediaComplete(cp) {
		if !fresh {
			if err = m.inspectMediaResume(ctx, j, &cp); err != nil {
				return m.mediaFailure(j, err)
			}
		}
		if err = m.transition(ctx, j, "downloading"); err != nil {
			return err
		}
		if err = m.downloadMedia(ctx, j, &cp); err != nil {
			return m.mediaFailure(j, err)
		}
	}
	if err = m.transition(ctx, j, "assembling"); err != nil {
		return err
	}
	return m.assembleMedia(ctx, j, cp)
}
func (m *Manager) mediaFailure(j *Job, err error) error {
	category := "media_transfer"
	if j.State == "assembling" || j.State == "verifying" {
		category = "media_assembly"
	}
	attention := false
	switch {
	case errors.Is(err, errMediaOwnership):
		category = "media_local"
		attention = true
	case errors.Is(err, engine.ErrUnsafeResume):
		category = "media_identity"
		attention = true
	case errors.Is(err, media.ErrUnsupported), errors.Is(err, media.ErrLimit):
		category = "media_unsupported"
		attention = true
	case errors.Is(err, media.ErrDependency):
		category = "media_dependency"
	case errors.Is(err, media.ErrAssembly):
		category = "media_assembly"
	}
	return m.fail(j, err, category, attention)
}

type objectResult struct {
	index  int
	object mediaObject
	err    error
}

func downloadMediaObject(ctx context.Context, client *http.Client, source string, r mediaObject, f *os.File) (result mediaObject, err error) {
	result = r
	defer func() { err = errors.Join(err, f.Close()) }()
	if err = f.Truncate(0); err != nil {
		return result, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	h := sha256.New()
	result.Remote, err = media.DownloadObject(ctx, client, source, io.MultiWriter(f, h), nil)
	if err != nil {
		return result, err
	}
	info, err := f.Stat()
	if err != nil {
		return result, err
	}
	if info.Size() == 0 {
		return result, media.ErrUnsupported
	}
	if err = f.Sync(); err != nil {
		return result, err
	}
	result.Length = info.Size()
	result.Digest = hex.EncodeToString(h.Sum(nil))
	result.Complete = true
	return result, nil
}
func (m *Manager) downloadMedia(ctx context.Context, j *Job, cp *mediaCheckpoint) error {
	var pending []int
	for i, r := range cp.Objects {
		if !r.Complete {
			pending = append(pending, i)
		}
	}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		count := min(m.config.Workers, len(pending))
		indices := pending[:count]
		pending = pending[count:]
		files := make([]*os.File, 0, count)
		var failure error
		for _, index := range indices {
			r := cp.Objects[index]
			var f *os.File
			var err error
			if r.File == "" {
				var a allocation
				f, a, err = allocateOwnedFile(cp.Directory, fmt.Sprintf("segment-%06d-*.ts", index))
				r.File, r.Identity, r.Ownership = a.Path, a.Identity, a.Ownership
				cp.Objects[index] = r
			} else {
				f, err = openOwnedFile(r.File, r.Identity, r.Ownership, os.O_RDWR)
				if err != nil {
					err = errMediaOwnership
				}
			}
			if f != nil {
				files = append(files, f)
			}
			if err != nil {
				failure = err
				break
			}
		}
		if failure == nil {
			failure = m.saveMedia(ctx, j, *cp)
		}
		if failure != nil {
			for _, f := range files {
				failure = errors.Join(failure, f.Close())
			}
			return failure
		}
		// Allocation ownership is committed before workers can mutate any resource.
		window, cancel := context.WithCancel(ctx)
		results := make(chan objectResult, count)
		for position, index := range indices {
			go func() {
				r, err := downloadMediaObject(window, m.client, cp.Plan.Segments[index], cp.Objects[index], files[position])
				results <- objectResult{index, r, err}
			}()
		}
		for range count {
			result := <-results
			if result.err != nil {
				// Cancelling peers after a failure is cleanup, not a new retry
				// veto. Preserve any joined disk/close error and caller cancellation.
				if failure != nil && ctx.Err() == nil && mediaPeerCancelled(result.err) {
					continue
				}
				failure = errors.Join(failure, result.err)
				cancel()
				continue
			}
			cp.Objects[result.index] = result.object
		}
		cancel()
		// Every worker has closed its file. Only synced complete objects enter the
		// checkpoint, and uncertain/incomplete bodies are redownloaded in full.
		if err := m.step("after-media-file-sync"); err != nil {
			return errors.Join(failure, err)
		}
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		checkpointErr := m.saveMedia(cleanup, j, *cp)
		stop()
		if checkpointErr != nil {
			return errors.Join(failure, checkpointErr)
		}
		if err := m.step("after-media-checkpoint"); err != nil {
			return errors.Join(failure, err)
		}
		if failure != nil {
			return failure
		}
	}
	return nil
}

func mediaPeerCancelled(err error) bool {
	if err == context.Canceled {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !mediaPeerCancelled(cause) {
				return false
			}
		}
		return true
	}
	if cause := errors.Unwrap(err); cause != nil {
		return mediaPeerCancelled(cause)
	}
	return false
}

// mediaInput verifies each owned object's bytes as they are consumed. At most
// one resource is open; no untrusted filename or protocol reaches FFmpeg.
type mediaInput struct {
	ctx      context.Context
	objects  []mediaObject
	next     int
	file     *os.File
	hash     hash.Hash
	position int64
}

func (r *mediaInput) Close() error {
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}
func (r *mediaInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.next >= len(r.objects) {
		return 0, io.EOF
	}
	object := r.objects[r.next]
	if r.file == nil {
		f, err := openOwnedFile(object.File, object.Identity, object.Ownership, os.O_RDONLY)
		if err != nil {
			return 0, errMediaOwnership
		}
		r.file = f
		r.hash = sha256.New()
		r.position = 0
	}
	n, err := r.file.Read(p)
	if n > 0 {
		_, _ = r.hash.Write(p[:n])
		r.position += int64(n)
	}
	if r.position > object.Length {
		return n, errMediaOwnership
	}
	if errors.Is(err, io.EOF) {
		if r.position != object.Length || hex.EncodeToString(r.hash.Sum(nil)) != object.Digest {
			return n, errMediaOwnership
		}
		if closeErr := r.Close(); closeErr != nil {
			return n, closeErr
		}
		r.next++
		if n > 0 {
			return n, nil
		}
		return r.Read(p)
	}
	return n, err
}
func (m *Manager) assembleMedia(ctx context.Context, j *Job, cp mediaCheckpoint) (err error) {
	var inputBytes int64
	for _, r := range cp.Objects {
		inputBytes += r.Length
	}
	reader := &mediaInput{ctx: ctx, objects: cp.Objects}
	defer func() { err = errors.Join(err, reader.Close()) }()
	var output *os.File
	if j.Partial == "" {
		output, err = m.allocate(ctx, j)
	} else {
		output, err = owned(*j, os.O_RDWR)
		if err != nil {
			err = errMediaOwnership
		}
	}
	if err != nil {
		return m.mediaFailure(j, err)
	}
	defer func() {
		if output != nil {
			err = errors.Join(err, output.Close())
		}
	}()
	assemble := m.assemble
	if assemble == nil {
		assemble = media.Assemble
	}
	if err = m.step("before-media-assembly"); err != nil {
		return err
	}
	if err = assemble(ctx, reader, inputBytes, output); err != nil {
		return m.mediaFailure(j, err)
	}
	if reader.next != len(cp.Objects) || reader.file != nil {
		return m.mediaFailure(j, media.ErrAssembly)
	}
	if err = m.step("after-media-assembly"); err != nil {
		return err
	}
	if err = m.transition(ctx, j, "verifying"); err != nil {
		return err
	}
	if _, err = output.Seek(0, io.SeekStart); err != nil {
		return m.mediaFailure(j, err)
	}
	j.Digest, err = digest(contextReader{ctx: ctx, reader: output})
	if err != nil {
		return m.mediaFailure(j, err)
	}
	info, err := output.Stat()
	if err != nil {
		return m.mediaFailure(j, err)
	}
	j.Length = info.Size()
	j.Durable = j.Length
	if err = output.Sync(); err != nil {
		return m.mediaFailure(j, err)
	}
	err = output.Close()
	output = nil
	if err != nil {
		return m.mediaFailure(j, err)
	}
	if err = ctx.Err(); err != nil {
		return m.mediaFailure(j, err)
	}
	return m.finishVerified(ctx, j)
}

// Explicit restart may abandon unproven files, exactly as the file reset path
// does. Cancellation/removal never acknowledges deletion of unproven objects.
func (m *Manager) discardMedia(j *Job) error { return m.cleanMedia(j, false) }
func (m *Manager) cleanMedia(j *Job, abandon bool) error {
	if len(j.Media) == 0 {
		return nil
	}
	cp, err := decodeMedia(*j)
	if err != nil {
		if abandon {
			return nil
		}
		return err
	}
	if filepath.Dir(cp.Directory) == m.directory && strings.HasPrefix(filepath.Base(cp.Directory), "media-"+j.ID+"-") {
		if _, e := os.Lstat(cp.Directory); errors.Is(e, os.ErrNotExist) {
			return nil
		}
	}
	if err = m.checkMediaDirectory(*j, cp); err != nil {
		if abandon {
			return nil
		}
		return err
	}
	for _, r := range cp.Objects {
		if r.File == "" {
			continue
		}
		if _, err = os.Lstat(r.File); errors.Is(err, os.ErrNotExist) {
			continue
		}
		f, err := openOwnedFile(r.File, r.Identity, r.Ownership, os.O_RDONLY)
		if err != nil {
			if abandon {
				continue
			}
			return errMediaOwnership
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = os.Remove(r.File); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(cp.Directory)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		if abandon {
			return nil
		}
		return errMediaOwnership
	}
	if err = os.Remove(cp.Directory); err != nil {
		return err
	}
	return localfs.SyncDir(m.directory)
}

// MediaSummary contains no source URLs, cache paths, validators or ownership.
type MediaSummary struct {
	Objects, Complete int
	Bytes             int64
}

func MediaProgress(j Job) MediaSummary {
	if j.Kind != "hls" || j.MediaObjects < 0 || j.MediaObjects > media.MaxObjects || j.MediaComplete < 0 || j.MediaComplete > j.MediaObjects || j.MediaBytes < 0 || j.MediaBytes > media.MaxObjectBytes*int64(j.MediaObjects) {
		return MediaSummary{}
	}
	return MediaSummary{Objects: j.MediaObjects, Complete: j.MediaComplete, Bytes: j.MediaBytes}
}
func (m *Manager) AnalyzeMedia(ctx context.Context, source string) ([]media.Choice, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("manager is closed")
	}
	select {
	case m.analysisSlots <- struct{}{}:
	default:
		m.mu.Unlock()
		return nil, errors.New("too many media analyses")
	}
	client := *m.client
	m.mu.Unlock()
	defer func() { <-m.analysisSlots }()
	return media.Analyze(ctx, &client, source)
}
func (s *Session) AnalyzeMedia(ctx context.Context, source string) ([]media.Choice, error) {
	s.commands.Lock()
	if err := s.available(); err != nil {
		s.commands.Unlock()
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.analysisContext, cancel)
	s.analyses.Add(1)
	s.commands.Unlock()
	defer func() { stop(); cancel(); s.analyses.Done() }()
	return s.manager.AnalyzeMedia(ctx, source)
}
