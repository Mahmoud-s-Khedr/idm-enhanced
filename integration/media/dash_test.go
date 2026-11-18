//go:build media

package media_test

import (
	"context"
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
