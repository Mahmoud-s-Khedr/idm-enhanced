//go:build media

package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
)

func TestRealHLSVariantsAndOfflineAssemblyRetry(t *testing.T) {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("required media fixture tool %s unavailable: %v", name, err)
		}
	}
	root, err := filepath.Abs("../../testdata/hls")
	if err != nil {
		t.Fatal(err)
	}
	for index, width := range []int{128, 256} {
		t.Run(widthName(width), func(t *testing.T) {
			var requests atomic.Int32
			files := http.FileServer(http.Dir(root))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); files.ServeHTTP(w, r) }))
			defer server.Close()
			m, err := manager.Open(t.TempDir(), false, server.Client())
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
			choices, err := m.AnalyzeMedia(ctx, server.URL+"/master.m3u8?token=private")
			if err != nil || len(choices) != 2 {
				t.Fatal(choices, err)
			}
			destination := filepath.Join(t.TempDir(), "playable.mp4")
			j, err := m.Create(ctx, manager.Options{Kind: "hls", MediaChoice: choices[index].ID, URL: server.URL + "/master.m3u8?token=private", Destination: destination})
			if err != nil {
				t.Fatal(err)
			}
			// Simulate installing the optional tools after resources have downloaded.
			executablePath := os.Getenv("PATH")
			t.Setenv("PATH", t.TempDir())
			if err = m.Run(ctx, j.ID, false, nil); err == nil {
				t.Fatal("missing FFmpeg reported completion")
			}
			jobs, err := m.Status(ctx)
			if err != nil || len(jobs) != 1 || jobs[0].State != "failed" || manager.MediaProgress(jobs[0]).Complete != 2 {
				t.Fatal(jobs, err)
			}
			count := requests.Load()
			server.Close()
			t.Setenv("PATH", executablePath)
			if err = m.Run(ctx, j.ID, false, nil); err != nil {
				t.Fatal("offline assembly failed", err)
			}
			if requests.Load() != count {
				t.Fatal("assembly retried source downloads")
			}
			probe := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,width,height:format=duration", "-of", "json", destination)
			data, err := probe.Output()
			if err != nil {
				t.Fatal(err)
			}
			var info struct {
				Streams []struct {
					Type  string `json:"codec_type"`
					Width int    `json:"width"`
				}
				Format struct {
					Duration string `json:"duration"`
				}
			}
			if err = json.Unmarshal(data, &info); err != nil {
				t.Fatal(err)
			}
			video := false
			for _, stream := range info.Streams {
				if stream.Type == "video" && stream.Width == width {
					video = true
				}
			}
			if !video {
				t.Fatalf("wrong selected video: %s", data)
			}
			var stderr bytes.Buffer
			decode := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", destination, "-f", "null", "-")
			decode.Stdout = io.Discard
			decode.Stderr = &stderr
			if err = decode.Run(); err != nil {
				t.Fatalf("fixture does not play: %v %s", err, stderr.String())
			}
			jobs, err = m.Status(ctx)
			if err != nil || jobs[0].State != "completed" {
				t.Fatal(jobs, err)
			}
		})
	}
}
func widthName(width int) string {
	if width == 128 {
		return "low"
	}
	return "high"
}
