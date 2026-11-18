//go:build media

package media_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/media"
)

func TestRealDASHSelectedTracksAndOfflineAssembly(t *testing.T) {
	root, err := filepath.Abs("../../testdata/dash")
	if err != nil {
		t.Fatal(err)
	}
	for index := range 4 {
		t.Run(fmt.Sprintf("choice-%d", index), func(t *testing.T) {
			var mu sync.Mutex
			requests := map[string]int{}
			files := http.FileServer(http.Dir(root))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests[r.URL.Path]++
				mu.Unlock()
				files.ServeHTTP(w, r)
			}))
			defer server.Close()
			state := t.TempDir()
			m, err := manager.Open(state, false, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			source := server.URL + "/manifest.mpd?token=private"
			choices, err := m.AnalyzeMedia(ctx, source)
			if err != nil || len(choices) != 4 {
				t.Fatal(choices, err)
			}
			destination := filepath.Join(t.TempDir(), "selected.mp4")
			j, err := m.Create(ctx, manager.Options{Kind: "dash", MediaChoice: choices[index].ID, URL: source, Destination: destination})
			if err != nil {
				t.Fatal(err)
			}
			executablePath := os.Getenv("PATH")
			t.Setenv("PATH", t.TempDir())
			if err = m.Run(ctx, j.ID, false, nil); err == nil {
				t.Fatal("missing tools completed DASH")
			}
			jobs, err := m.Status(ctx)
			if err != nil || jobs[0].State != "failed" || manager.MediaProgress(jobs[0]).Complete != 6 {
				t.Fatal(jobs, err)
			}
			mu.Lock()
			for track := range 4 {
				expected := track == index/2 || track == 2+index%2
				if (requests[fmt.Sprintf("/init-%d.mp4", track)] == 1) != expected {
					t.Fatal("downloaded unselected track", requests)
				}
			}
			count := 0
			for _, n := range requests {
				count += n
			}
			mu.Unlock()
			server.Close()
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			m, err = manager.Open(state, false, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", executablePath)
			if err = m.Run(ctx, j.ID, false, nil); err != nil {
				t.Fatal("offline DASH assembly", err)
			}
			probe := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_name,width:format=duration", "-of", "json", destination)
			data, err := probe.Output()
			width := 128
			if index >= 2 {
				width = 256
			}
			if err != nil || !strings.Contains(string(data), fmt.Sprintf(`"width": %d`, width)) || !strings.Contains(string(data), `"aac"`) || !strings.Contains(string(data), `"h264"`) {
				t.Fatalf("wrong selected playback %s: %v", data, err)
			}
			decode := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", destination, "-f", "null", "-")
			if output, err := decode.CombinedOutput(); err != nil {
				t.Fatalf("DASH playback failed %s: %v", output, err)
			}
			jobs, err = m.Status(ctx)
			if err != nil || jobs[0].State != "completed" {
				t.Fatal(jobs, err)
			}
			mu.Lock()
			after := 0
			for _, n := range requests {
				after += n
			}
			mu.Unlock()
			if after != count {
				t.Fatal("offline assembly contacted origin")
			}
			if err = m.Remove(ctx, j.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(destination); err != nil {
				t.Fatal("history removal deleted output", err)
			}
		})
	}
}

func TestRealDASHRejectsPresentationTailBeforePublication(t *testing.T) {
	root, err := filepath.Abs("../../testdata/dash")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "manifest.mpd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"terminal-overlap", "encoded-duration"} {
		t.Run(mode, func(t *testing.T) {
			body := strings.Replace(string(data), "PT2.0S", "PT1.5S", 1)
			if mode == "encoded-duration" {
				body = strings.NewReplacer("PT2.0S", "PT1.0S", `d="10240" r="1"`, `d="5120" r="1"`, `d="44032"`, `d="22016"`, `d="44168"`, `d="22084"`).Replace(string(data))
			}
			files := http.FileServer(http.Dir(root))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/bad.mpd" {
					http.ServeContent(w, r, "manifest", time.Time{}, strings.NewReader(body))
					return
				}
				files.ServeHTTP(w, r)
			}))
			defer server.Close()
			m, err := manager.Open(t.TempDir(), false, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.Close() }()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			destination := filepath.Join(t.TempDir(), "must-not-publish.mp4")
			j, err := m.Create(ctx, manager.Options{Kind: "dash", URL: server.URL + "/bad.mpd", Destination: destination})
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Run(ctx, j.ID, false, nil); !errors.Is(err, media.ErrUnsupported) {
				t.Fatal("accepted incompatible presentation", err)
			}
			if _, err = os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("published presentation tail", err)
			}
			jobs, err := m.Status(ctx)
			if err != nil || len(jobs) != 1 || jobs[0].State != "needs_attention" {
				t.Fatal(jobs, err)
			}
		})
	}
}
