package manager

import (
	"context"
	"errors"
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
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

const dashMPD = `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT1S"><Period><AdaptationSet mimeType="video/mp4" codecs="avc1.42c01e" width="128" height="72"><SegmentTemplate duration="1" initialization="init-$RepresentationID$" media="$RepresentationID$-$Number$"/><Representation id="v" bandwidth="1000"/></AdaptationSet><AdaptationSet mimeType="audio/mp4" codecs="mp4a.40.2" lang="eng"><SegmentTemplate duration="1" initialization="init-$RepresentationID$" media="$RepresentationID$-$Number$"/><Representation id="a" bandwidth="500"/></AdaptationSet></Period></MPD>`

func fakeDASHAssembly(ctx context.Context, tracks []media.TrackInput, out *os.File) error {
	var all strings.Builder
	for _, track := range tracks {
		if _, err := io.Copy(&all, track.Reader); err != nil {
			return err
		}
	}
	return fakeMediaAssembly(ctx, strings.NewReader(all.String()), int64(all.Len()), out)
}
func dashFixture(t *testing.T) (*Manager, *httptest.Server, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	var fail atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasPrefix(r.URL.Path, "/index") {
			_, _ = io.WriteString(w, dashMPD)
			return
		}
		if fail.Load() && r.URL.Path == "/v-1" {
			w.WriteHeader(503)
			return
		}
		data := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("ETag", `"v1"`)
		http.ServeContent(w, r, "object", time.Time{}, strings.NewReader(data))
	}))
	t.Cleanup(server.Close)
	m, err := Open(t.TempDir(), false, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	configureMediaFixture(t, m, server.Client())
	m.assembleTracks = fakeDASHAssembly
	t.Cleanup(func() { _ = m.Close() })
	return m, server, &fail, &calls
}
func dashJob(t *testing.T, m *Manager, server *httptest.Server) Job {
	t.Helper()
	j, err := m.Create(context.Background(), Options{Kind: "dash", URL: server.URL + "/index.mpd?private=yes", Destination: filepath.Join(t.TempDir(), "output.mp4")})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func assertDASHOutput(t *testing.T, j Job) {
	t.Helper()
	data, err := os.ReadFile(j.Destination)
	if err != nil || string(data) != "assembled:init-vv-1init-aa-1" {
		t.Fatal(string(data), err)
	}
}
func TestDASHResumeOfflineMuxAndCleanup(t *testing.T) {
	m, server, fail, calls := dashFixture(t)
	j := dashJob(t, m, server)
	fail.Store(true)
	ctx := context.Background()
	if err := m.Run(ctx, j.ID, false, nil); err == nil {
		t.Fatal("ignored failed fragment")
	}
	saved := mediaSaved(t, m, j.ID)
	if MediaProgress(saved).Complete != 1 || saved.State != "failed" {
		t.Fatal(saved)
	}
	cp, err := decodeMedia(saved)
	if err != nil || cp.Version != 2 || len(cp.Plan.Tracks) != 2 {
		t.Fatal(cp, err)
	}
	fail.Store(false)
	m.assembleTracks = func(context.Context, []media.TrackInput, *os.File) error { return media.ErrDependency }
	if err = m.Run(ctx, j.ID, false, nil); !errors.Is(err, media.ErrDependency) {
		t.Fatal(err)
	}
	saved = mediaSaved(t, m, j.ID)
	if MediaProgress(saved).Complete != 4 {
		t.Fatal(saved)
	}
	count := calls.Load()
	server.Close()
	directory := m.directory
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = Open(directory, false, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	m.assembleTracks = fakeDASHAssembly
	if err = m.Run(ctx, j.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != count {
		t.Fatal("offline retry contacted origin")
	}
	assertDASHOutput(t, j)
	if err = m.Remove(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(cp.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	assertDASHOutput(t, j)
}
func TestDASHCheckpointTrackOwnershipAndBrowserAdmission(t *testing.T) {
	m, server, _, _ := dashFixture(t)
	ctx := context.Background()
	request := protocol.Message{Version: 1, ID: "dash-receipt", Type: "download", Mode: "dash", URL: server.URL + "/index.mpd", Filename: "manifest.mpd"}
	id, err := m.AcceptBrowser(ctx, request, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if same, e := m.AcceptBrowser(ctx, request, t.TempDir()); e != nil || same != id {
		t.Fatal(same, e)
	}
	request.Mode = "hls"
	if _, e := m.AcceptBrowser(ctx, request, t.TempDir()); e == nil {
		t.Fatal("changed receipt semantics")
	}
	j := mediaSaved(t, m, id)
	if j.Kind != "dash" || !strings.HasSuffix(j.Destination, "manifest.mp4") {
		t.Fatal(j)
	}
	m.assembleTracks = func(context.Context, []media.TrackInput, *os.File) error { return media.ErrDependency }
	if err = m.Run(ctx, id, false, nil); !errors.Is(err, media.ErrDependency) {
		t.Fatal(err)
	}
	j = mediaSaved(t, m, id)
	cp, err := decodeMedia(j)
	if err != nil {
		t.Fatal(err)
	}
	for _, tracks := range [][]media.Track{nil, {{Kind: "subtitle", End: 4}}, {{Kind: "video", End: 1}}, {{Kind: "video", End: 5}}, {{Kind: "video", End: 2}, {Kind: "video", End: 4}}, {{Kind: "video", End: 2}}} {
		bad := cp.Plan
		bad.Tracks = tracks
		if validateMediaTracks("dash", bad, 4) == nil {
			t.Fatal("accepted corrupt track boundaries", tracks)
		}
	}
	bad := cp.Plan
	bad.Tracks = cp.Plan.Tracks
	if validateMediaTracks("hls", bad, 4) == nil || validateMediaTracks("unknown", bad, 4) == nil {
		t.Fatal("accepted wrong checkpoint kind")
	}
	if err = validateMediaTracks("dash", cp.Plan, 4); err != nil {
		t.Fatal(err)
	}
	if !sameMediaTracks(cp.Plan.Tracks, cp.Plan.Tracks) || sameMediaTracks(cp.Plan.Tracks, nil) || sameMediaTracks(cp.Plan.Tracks, []media.Track{{Kind: "audio", End: 2}, {Kind: "video", End: 4}}) {
		t.Fatal("track equality")
	}
	if err = m.Cancel(ctx, id, false); err != nil {
		t.Fatal(err)
	}
}

func TestDASHNeverPublishesUnconsumedOrDamagedTracks(t *testing.T) {
	for _, mode := range []string{"unconsumed", "damaged-audio"} {
		t.Run(mode, func(t *testing.T) {
			m, server, _, _ := dashFixture(t)
			j := dashJob(t, m, server)
			ctx := context.Background()
			m.assembleTracks = func(context.Context, []media.TrackInput, *os.File) error { return media.ErrDependency }
			if err := m.Run(ctx, j.ID, false, nil); !errors.Is(err, media.ErrDependency) {
				t.Fatal(err)
			}
			server.Close()
			if mode == "unconsumed" {
				m.assembleTracks = func(_ context.Context, _ []media.TrackInput, out *os.File) error {
					_, err := io.WriteString(out, "unverified")
					return err
				}
			} else {
				saved := mediaSaved(t, m, j.ID)
				cp, err := decodeMedia(saved)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(cp.Objects[2].File, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
				m.assembleTracks = fakeDASHAssembly
			}
			if err := m.Run(ctx, j.ID, false, nil); err == nil {
				t.Fatal("published unverified media")
			}
			if _, err := os.Stat(j.Destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("published bad output", err)
			}
			saved := mediaSaved(t, m, j.ID)
			if mode == "damaged-audio" && saved.State != "needs_attention" {
				t.Fatal(saved.State, saved.Error)
			}
		})
	}
}
