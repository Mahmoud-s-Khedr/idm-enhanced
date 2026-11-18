package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/media"
)

const mediaVOD = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:1,\none.ts\n#EXTINF:1,\ntwo.ts\n#EXTINF:1,\nthree.ts\n#EXT-X-ENDLIST\n"

func fakeMediaAssembly(ctx context.Context, input io.Reader, _ int64, output *os.File) error {
	if err := output.Truncate(0); err != nil {
		return err
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := output.WriteString("assembled:"); err != nil {
		return err
	}
	_, err := io.Copy(output, contextReader{ctx: ctx, reader: input})
	return err
}

type mediaOrigin struct {
	fail, changed, etag, weak atomic.Bool
	calls                     [3]atomic.Int32
	requests                  atomic.Int32
}

func (o *mediaOrigin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.requests.Add(1)
	if r.URL.Path == "/index.m3u8" {
		text := mediaVOD
		if o.changed.Load() {
			text += "# different snapshot\n"
		}
		_, _ = io.WriteString(w, text)
		return
	}
	for i, name := range []string{"one", "two", "three"} {
		if r.URL.Path == "/"+name+".ts" {
			if r.Header.Get("Range") == "" {
				o.calls[i].Add(1)
			}
			if i == 1 && o.fail.Load() {
				w.WriteHeader(503)
				return
			}
			if !o.weak.Load() {
				etag := `"v1"`
				if o.etag.Load() {
					etag = `"v2"`
				}
				w.Header().Set("ETag", etag)
			}
			http.ServeContent(w, r, name, time.Time{}, strings.NewReader(name))
			return
		}
	}
	w.WriteHeader(404)
}
func mediaFixture(t *testing.T) (*Manager, *httptest.Server, *mediaOrigin) {
	t.Helper()
	origin := &mediaOrigin{}
	server := httptest.NewServer(origin)
	t.Cleanup(server.Close)
	m, err := Open(t.TempDir(), false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	configureMediaFixture(t, m, server.Client())
	t.Cleanup(func() { _ = m.Close() })
	return m, server, origin
}
func configureMediaFixture(t *testing.T, m *Manager, client *http.Client) {
	t.Helper()
	config := DefaultConfig()
	config.Workers = 1
	config.Retries = 0
	if err := m.Configure(config, client); err != nil {
		t.Fatal(err)
	}
	m.assemble = fakeMediaAssembly
}
func mediaJob(t *testing.T, m *Manager, server *httptest.Server) Job {
	t.Helper()
	j, err := m.Create(context.Background(), Options{Kind: "hls", URL: server.URL + "/index.m3u8?token=private", Destination: filepath.Join(t.TempDir(), "output.mp4")})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func mediaSaved(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	j, err := m.store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func assertMediaOutput(t *testing.T, j Job) {
	t.Helper()
	data, err := os.ReadFile(j.Destination)
	if err != nil || string(data) != "assembled:onetwothree" {
		t.Fatal(string(data), err)
	}
}

func TestMediaResumeKeepsSyncedObjectsAndPublishesInOrder(t *testing.T) {
	m, server, origin := mediaFixture(t)
	origin.fail.Store(true)
	j := mediaJob(t, m, server)
	ctx := context.Background()
	if err := m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("ignored failed object")
	}
	saved := mediaSaved(t, m, j.ID)
	progress := MediaProgress(saved)
	if saved.State != "failed" || progress.Objects != 3 || progress.Complete != 1 || progress.Bytes != 3 {
		t.Fatal(saved.State, progress)
	}
	listed, err := m.Status(ctx)
	if err != nil || len(listed) != 1 || len(listed[0].Media) != 0 || MediaProgress(listed[0]) != progress {
		t.Fatal("polling loaded detailed checkpoints", listed, err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(m.directory, false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	configureMediaFixture(t, reopened, server.Client())
	origin.fail.Store(false)
	if err = reopened.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	saved = mediaSaved(t, reopened, j.ID)
	if saved.State != "completed" || saved.Durable != int64(len("assembled:onetwothree")) || origin.calls[0].Load() != 1 {
		t.Fatal(saved.State, saved.Durable, origin.calls[0].Load())
	}
	assertMediaOutput(t, saved)
	cp, err := decodeMedia(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.Remove(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(cp.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("media cache retained after removal", err)
	}
	assertMediaOutput(t, saved)
}
func TestMediaAssemblyRetryNeedsNoNetworkAndPreservesObjects(t *testing.T) {
	for _, failure := range []error{media.ErrAssembly, media.ErrDependency} {
		t.Run(failure.Error(), func(t *testing.T) {
			m, server, origin := mediaFixture(t)
			j := mediaJob(t, m, server)
			m.assemble = func(context.Context, io.Reader, int64, *os.File) error { return failure }
			if err := m.Run(context.Background(), j.ID, false, nil); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			saved := mediaSaved(t, m, j.ID)
			if saved.State != "failed" || MediaProgress(saved).Complete != 3 {
				t.Fatal(saved.State, MediaProgress(saved))
			}
			requests := origin.requests.Load()
			server.Close()
			m.assemble = fakeMediaAssembly
			if err := m.Run(context.Background(), j.ID, false, nil); err != nil {
				t.Fatal(err)
			}
			if origin.requests.Load() != requests {
				t.Fatal("assembly retry contacted the origin")
			}
			assertMediaOutput(t, j)
		})
	}
}
func TestMediaUnsafeSourceOrCacheRequiresExplicitRestart(t *testing.T) {
	for _, mode := range []string{"manifest", "remote identity", "damaged object", "replaced object", "missing object", "foreign cache directory", "replaced cache directory", "cache path is a file"} {
		t.Run(mode, func(t *testing.T) {
			m, server, origin := mediaFixture(t)
			origin.fail.Store(true)
			j := mediaJob(t, m, server)
			ctx := context.Background()
			if err := m.Run(ctx, j.ID, false, nil); err == nil {
				t.Fatal("expected interruption")
			}
			saved := mediaSaved(t, m, j.ID)
			cp, err := decodeMedia(saved)
			if err != nil {
				t.Fatal(err)
			}
			path := cp.Objects[0].File
			switch mode {
			case "manifest":
				origin.changed.Store(true)
			case "remote identity":
				origin.etag.Store(true)
			case "damaged object":
				if err = os.WriteFile(path, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replaced object":
				if err = os.Rename(path, path+".retained"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing object":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "replaced cache directory", "cache path is a file":
				if err = os.Rename(cp.Directory, cp.Directory+".retained"); err != nil {
					t.Fatal(err)
				}
				if mode == "replaced cache directory" {
					err = os.Mkdir(cp.Directory, 0700)
				} else {
					err = os.WriteFile(cp.Directory, []byte("foreign"), 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "foreign cache directory":
				cp.Directory = filepath.Dir(j.Destination)
				if err = m.saveMedia(ctx, &saved, cp); err != nil {
					t.Fatal(err)
				}
			}
			origin.fail.Store(false)
			if err = m.Run(ctx, j.ID, false, nil); err == nil {
				t.Fatal("unsafe resume succeeded")
			}
			saved = mediaSaved(t, m, j.ID)
			if saved.State != "needs_attention" {
				t.Fatal(saved.State)
			}
			if _, err = os.Stat(j.Destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe output published", err)
			}
			if err = m.Run(ctx, j.ID, true, nil); err != nil {
				t.Fatal("explicit restart failed", err)
			}
			assertMediaOutput(t, j)
			if mode == "replaced object" {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "foreign" {
					t.Fatal("foreign object removed", err)
				}
			}
		})
	}
}
func TestMediaWeakObjectsRedownloadAndCancellationRetention(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprint(keep), func(t *testing.T) {
			m, server, origin := mediaFixture(t)
			origin.weak.Store(true)
			origin.fail.Store(true)
			j := mediaJob(t, m, server)
			ctx := context.Background()
			if m.Run(ctx, j.ID, false, nil) == nil {
				t.Fatal("expected failure")
			}
			saved := mediaSaved(t, m, j.ID)
			cp, err := decodeMedia(saved)
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Cancel(ctx, j.ID, keep); err != nil {
				t.Fatal(err)
			}
			origin.fail.Store(false)
			if !keep {
				if m.Run(ctx, j.ID, false, nil) == nil {
					t.Fatal("discarded media resumed without restart")
				}
				if _, err = os.Stat(cp.Directory); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("discard retained cache", err)
				}
			}
			if err = m.Run(ctx, j.ID, !keep, nil); err != nil {
				t.Fatal(err)
			}
			assertMediaOutput(t, j)
			if origin.calls[0].Load() != 2 {
				t.Fatal("weak/explicitly discarded object reused", origin.calls[0].Load())
			}
		})
	}
}
func TestMediaCheckpointBoundariesNeverClaimUnsyncedObjects(t *testing.T) {
	for _, boundary := range []string{"after-media-file-sync", "after-media-checkpoint", "before-media-assembly", "after-media-assembly"} {
		t.Run(boundary, func(t *testing.T) {
			m, server, origin := mediaFixture(t)
			j := mediaJob(t, m, server)
			m.boundary = func(step string) error {
				if step == boundary {
					return errors.New("fixture interruption")
				}
				return nil
			}
			if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("boundary ignored")
			}
			saved := mediaSaved(t, m, j.ID)
			expected := 1
			if boundary == "after-media-file-sync" {
				expected = 0
			}
			if strings.Contains(boundary, "assembly") {
				expected = 3
			}
			if MediaProgress(saved).Complete != expected {
				t.Fatal(MediaProgress(saved), expected)
			}
			m.boundary = nil
			if err := m.Run(context.Background(), j.ID, false, nil); err != nil {
				t.Fatal(err)
			}
			want := int32(1)
			if boundary == "after-media-file-sync" {
				want = 2
			}
			if origin.calls[0].Load() != want {
				t.Fatal("incorrect recovery reuse", origin.calls[0].Load(), want)
			}
			assertMediaOutput(t, j)
		})
	}
}
func TestMediaPauseDropsUncommittedObjectBytes(t *testing.T) {
	started := make(chan struct{})
	var first atomic.Bool
	first.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.m3u8" {
			_, _ = io.WriteString(w, mediaVOD)
			return
		}
		if r.URL.Path == "/one.ts" && first.Swap(false) {
			w.Header().Set("Content-Length", "3")
			_, _ = io.WriteString(w, "o")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			return
		}
		for _, name := range []string{"one", "two", "three"} {
			if r.URL.Path == "/"+name+".ts" {
				_, _ = io.WriteString(w, name)
				return
			}
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	m, err := Open(t.TempDir(), false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	configureMediaFixture(t, m, server.Client())
	j := mediaJob(t, m, server)
	done := make(chan error, 1)
	go func() { done <- m.Run(context.Background(), j.ID, false, nil) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("object never started")
	}
	if err = m.Pause(j.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pause did not join")
	}
	saved := mediaSaved(t, m, j.ID)
	if saved.State != "paused" || MediaProgress(saved).Complete != 0 {
		t.Fatal(saved.State, MediaProgress(saved))
	}
	if err = m.Run(context.Background(), j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	assertMediaOutput(t, j)
}
func TestMediaReceivingBoundaryAndCorruptCheckpoint(t *testing.T) {
	m, server, _ := mediaFixture(t)
	for _, o := range []Options{{Kind: "unknown", URL: server.URL, Destination: filepath.Join(t.TempDir(), "x")}, {Kind: "hls", URL: server.URL, Destination: filepath.Join(t.TempDir(), "x.ts")}, {Kind: "hls", URL: server.URL, Destination: filepath.Join(t.TempDir(), "x.mp4"), SHA256: strings.Repeat("a", 64)}, {Kind: "hls", URL: server.URL, Destination: filepath.Join(t.TempDir(), "x.mp4"), MediaChoice: "bad"}, {Kind: "hls", URL: server.URL, Destination: filepath.Join(t.TempDir(), "x.mp4"), MediaChoice: strings.Repeat("z", 64)}} {
		if _, err := m.Create(context.Background(), o); err == nil {
			t.Fatal("invalid media options accepted")
		}
	}
	j := mediaJob(t, m, server)
	j.Media = []byte(`{"Version":99}`)
	j.MediaLoaded = true
	if err := m.store.Save(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
		t.Fatal("unknown media checkpoint accepted")
	}
	if err := m.Run(context.Background(), j.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	assertMediaOutput(t, j)
	if progress := MediaProgress(Job{Kind: "hls", MediaObjects: -1}); progress != (MediaSummary{}) {
		t.Fatal(progress)
	}
	data, _ := json.Marshal(MediaProgress(mediaSaved(t, m, j.ID)))
	if bytes.Contains(data, []byte("private")) {
		t.Fatal("metadata exposed")
	}
}

func TestMediaAnalysisBoundsAndSessionSleepClose(t *testing.T) {
	started := make(chan struct{}, 4)
	var fast atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fast.Load() {
			_, _ = io.WriteString(w, mediaVOD)
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	s, _ := sessionFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.AnalyzeMedia(ctx, server.URL); results <- err }()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("analysis did not start")
		}
	}
	if _, err := s.AnalyzeMedia(ctx, server.URL); err == nil {
		t.Fatal("analysis queue unbounded")
	}
	if err := s.manager.Configure(DefaultConfig(), nil); err == nil {
		t.Fatal("limits changed during analysis")
	}
	if err := s.manager.Close(); err == nil {
		t.Fatal("manager closed with active analysis")
	}
	cancel()
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("analysis not joined")
		}
	}
	for _, action := range []string{"sleep", "pause", "close"} {
		go func() { _, err := s.AnalyzeMedia(context.Background(), server.URL); results <- err }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("analysis did not start")
		}
		switch action {
		case "sleep":
			s.Sleep()
		case "pause":
			s.PauseAll()
		case "close":
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case err := <-results:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("lifecycle did not cancel analysis")
		}
		if action == "sleep" {
			if _, err := s.AnalyzeMedia(context.Background(), server.URL); err == nil {
				t.Fatal("sleeping session admitted analysis")
			}
			s.Wake()
		}
		if action != "close" {
			fast.Store(true)
			if choices, err := s.AnalyzeMedia(context.Background(), server.URL); err != nil || len(choices) != 1 {
				t.Fatal(choices, err)
			}
			fast.Store(false)
		}
	}
	if _, err := s.AnalyzeMedia(context.Background(), server.URL); err == nil {
		t.Fatal("closed session admitted analysis")
	}
	if _, err := s.manager.AnalyzeMedia(context.Background(), server.URL); err == nil {
		t.Fatal("closed manager admitted analysis")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := s.manager.AnalyzeMedia(cancelled, server.URL); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestMediaCleanupIsIdempotentAndPreservesForeignFiles(t *testing.T) {
	m, server, origin := mediaFixture(t)
	j := mediaJob(t, m, server)
	m.assemble = func(context.Context, io.Reader, int64, *os.File) error { return media.ErrAssembly }
	if m.Run(context.Background(), j.ID, false, nil) == nil {
		t.Fatal("expected assembly failure")
	}
	saved := mediaSaved(t, m, j.ID)
	cp, err := decodeMedia(saved)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(cp.Directory, "foreign.txt")
	if err = os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Cancel(context.Background(), j.ID, false); !errors.Is(err, errMediaOwnership) {
		t.Fatal("foreign cache accepted for deletion", err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "keep" {
		t.Fatal("foreign data changed", err)
	}
	if err = os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	if err = m.Cancel(context.Background(), j.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = m.Remove(context.Background(), j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.store.Get(context.Background(), j.ID); err == nil {
		t.Fatal("history retained")
	}
	if origin.calls[0].Load() != 1 {
		t.Fatal("cleanup contacted origin")
	}
	// A crash after cache removal but before metadata deletion can safely retry.
	if err = m.cleanMedia(&saved, false); err != nil {
		t.Fatal("completed cleanup not idempotent", err)
	}
}

func TestMediaParallelRetryDoesNotMistakePeerCancellationForUserPause(t *testing.T) {
	started := make(chan struct{})
	var firstOne, firstTwo atomic.Bool
	firstOne.Store(true)
	firstTwo.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.m3u8":
			_, _ = io.WriteString(w, mediaVOD)
		case "/one.ts":
			if firstOne.Swap(false) {
				close(started)
				<-r.Context().Done()
				return
			}
			_, _ = io.WriteString(w, "one")
		case "/two.ts":
			if firstTwo.Swap(false) {
				select {
				case <-started:
				case <-r.Context().Done():
					return
				}
				w.WriteHeader(503)
				return
			}
			_, _ = io.WriteString(w, "two")
		case "/three.ts":
			_, _ = io.WriteString(w, "three")
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m, err := Open(t.TempDir(), false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	config := DefaultConfig()
	config.Workers = 2
	config.Retries = 1
	config.Backoff = time.Millisecond
	if err = m.Configure(config, server.Client()); err != nil {
		t.Fatal(err)
	}
	m.assemble = fakeMediaAssembly
	j := mediaJob(t, m, server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = m.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal("peer cleanup vetoed retry", err)
	}
	assertMediaOutput(t, j)
	if mediaPeerCancelled(errors.Join(context.Canceled, errors.New("disk close"))) {
		t.Fatal("joined disk error swallowed")
	}
	if !mediaPeerCancelled(fmt.Errorf("response: %w", context.Canceled)) {
		t.Fatal("wrapped cleanup cancellation not recognized")
	}
}

func TestMediaReadyCacheCannotFeedChangedBytesToFinalization(t *testing.T) {
	for _, mode := range []string{"append", "truncate", "replace", "unconsumed"} {
		t.Run(mode, func(t *testing.T) {
			m, server, _ := mediaFixture(t)
			j := mediaJob(t, m, server)
			m.assemble = func(context.Context, io.Reader, int64, *os.File) error { return media.ErrAssembly }
			if m.Run(context.Background(), j.ID, false, nil) == nil {
				t.Fatal("expected assembly failure")
			}
			saved := mediaSaved(t, m, j.ID)
			cp, err := decodeMedia(saved)
			if err != nil {
				t.Fatal(err)
			}
			path := cp.Objects[0].File
			switch mode {
			case "append":
				if err = os.WriteFile(path, []byte("one-extra"), 0600); err != nil {
					t.Fatal(err)
				}
			case "truncate":
				if err = os.WriteFile(path, []byte("o"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replace":
				if err = os.Rename(path, path+".retained"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m.assemble = fakeMediaAssembly
			if mode == "unconsumed" {
				m.assemble = func(_ context.Context, _ io.Reader, _ int64, f *os.File) error {
					_, err := f.WriteString("incomplete")
					return err
				}
			}
			if err = m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("published changed/unconsumed data")
			}
			if _, err = os.Stat(j.Destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed assembly published output", err)
			}
		})
	}
}
func TestMediaPublicationCollisionPreservesForeignOutput(t *testing.T) {
	m, server, _ := mediaFixture(t)
	j := mediaJob(t, m, server)
	m.assemble = func(ctx context.Context, input io.Reader, n int64, output *os.File) error {
		if err := fakeMediaAssembly(ctx, input, n, output); err != nil {
			return err
		}
		return os.WriteFile(j.Destination, []byte("foreign"), 0600)
	}
	if err := m.Run(context.Background(), j.ID, false, nil); err == nil {
		t.Fatal("collision was overwritten")
	}
	saved := mediaSaved(t, m, j.ID)
	if saved.State != "needs_attention" || saved.Error != "publication" {
		t.Fatal(saved.State, saved.Error)
	}
	if err := m.Cancel(context.Background(), j.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), j.ID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(j.Destination)
	if err != nil || string(data) != "foreign" {
		t.Fatal("foreign output changed", err)
	}
}
func TestMediaRejectsUnsupportedInitialSourcesAndEmptyObjects(t *testing.T) {
	for _, mode := range []string{"http failure", "live", "empty object"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "http failure" {
					w.WriteHeader(404)
					return
				}
				if r.URL.Path == "/index.m3u8" {
					body := mediaVOD
					if mode == "live" {
						body = strings.ReplaceAll(body, "#EXT-X-ENDLIST\n", "")
					}
					_, _ = io.WriteString(w, body)
				}
			}))
			defer server.Close()
			m, err := Open(t.TempDir(), false, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			configureMediaFixture(t, m, server.Client())
			j := mediaJob(t, m, server)
			if err = m.Run(context.Background(), j.ID, false, nil); err == nil {
				t.Fatal("unsupported source completed")
			}
			if _, err = os.Stat(j.Destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsupported output published", err)
			}
		})
	}
}

func TestMediaCancellationAroundVerifiedCommitCanResumeOffline(t *testing.T) {
	for _, point := range []string{"before-verified-commit", "after-verified-commit"} {
		t.Run(point, func(t *testing.T) {
			m, server, origin := mediaFixture(t)
			j := mediaJob(t, m, server)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.boundary = func(step string) error {
				if step == point {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			if err := m.Run(ctx, j.ID, false, nil); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			saved := mediaSaved(t, m, j.ID)
			if point == "before-verified-commit" && (saved.State != "paused" || saved.Phase == "verified") {
				t.Fatal("uncommitted output reported verified", saved.State, saved.Phase)
			}
			if point == "after-verified-commit" && saved.Phase != "verified" {
				t.Fatal("verified checkpoint lost", saved.Phase)
			}
			requests := origin.requests.Load()
			server.Close()
			m.boundary = nil
			if err := m.Run(context.Background(), j.ID, false, nil); err != nil {
				t.Fatal("finalization could not resume", err)
			}
			if requests != origin.requests.Load() {
				t.Fatal("finalization contacted source")
			}
			assertMediaOutput(t, j)
		})
	}
}

func TestMediaOutputAllocationFailureCanRetryOffline(t *testing.T) {
	m, server, origin := mediaFixture(t)
	j := mediaJob(t, m, server)
	directory := filepath.Dir(j.Destination)
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("ignored output allocation failure")
	}
	saved := mediaSaved(t, m, j.ID)
	if saved.State != "failed" || saved.Error != "media_assembly" || saved.MediaComplete != 3 || saved.Partial != "" {
		t.Fatal(saved.State, saved.Error, saved.MediaComplete, saved.Partial)
	}
	requests := origin.requests.Load()
	server.Close()
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if saved = mediaSaved(t, m, j.ID); saved.State != "completed" || origin.requests.Load() != requests {
		t.Fatal(saved.State, origin.requests.Load(), requests)
	}
	assertMediaOutput(t, saved)
}
