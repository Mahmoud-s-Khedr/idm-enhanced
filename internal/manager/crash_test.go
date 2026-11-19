package manager

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// This helper is only compiled into the test binary. Production has no fault env vars.
func TestCrashWorker(t *testing.T) {
	dir := os.Getenv("IDM_TEST_STATE")
	if dir == "" {
		return
	}
	m, err := Open(dir, false, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("IDM_TEST_MEDIA") == "1" {
		m.assemble = fakeMediaAssembly
		m.assembleTracks = fakeDASHAssembly
	}
	step := os.Getenv("IDM_TEST_STEP")
	if step != "" {
		m.boundary = func(name string) error {
			if name == step {
				fmt.Println("barrier")
				select {}
			}
			return nil
		}
	}
	err = m.Run(context.Background(), os.Getenv("IDM_TEST_JOB"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestProcessCrashRecovery(t *testing.T) {
	_, source := fixture(t)
	testCrashRecovery(t, source, []byte("hello"))
}
func TestParallelProcessCrashRecovery(t *testing.T) {
	data := bytes.Repeat([]byte("parallel"), 700000)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(data))
	}))
	defer s.Close()
	testCrashRecovery(t, s.URL, data)
}
func testCrashRecovery(t *testing.T, source string, expected []byte) {
	for _, step := range []string{"before-file-sync", "after-file-sync", "after-checkpoint", "before-verified-commit", "after-verified-commit", "after-publish", "after-remove", "after-completed"} {
		t.Run(step, func(t *testing.T) {
			dir := t.TempDir()
			m, err := Open(dir, false, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "result")
			j, err := m.Create(context.Background(), Options{URL: source, Destination: dest})
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := func(boundary string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorker$")
				cmd.Env = append(os.Environ(), "IDM_TEST_STATE="+dir, "IDM_TEST_JOB="+j.ID, "IDM_TEST_STEP="+boundary)
				return cmd
			}
			cmd := command(step)
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(pipe)
			if !scanner.Scan() || scanner.Text() != "barrier" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("worker did not reach crash boundary", scanner.Err())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			// Relaunch a fresh process; the OS must have released the owner lock.
			if output, err := command("").CombinedOutput(); err != nil {
				t.Fatalf("relaunch: %v %s", err, output)
			}
			data, err := os.ReadFile(dest)
			if err != nil || !bytes.Equal(data, expected) {
				t.Fatal("incorrect crash recovery", len(data), err)
			}
			ro, err := Open(dir, true, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			jobs, err := ro.Status(context.Background())
			if err != nil || jobs[0].State != "completed" {
				t.Fatal(jobs, err)
			}
			if err = ro.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKillWithActiveRangeWorkers(t *testing.T) {
	data := bytes.Repeat([]byte("active-range"), 500000)
	var stall atomic.Bool
	stall.Store(true)
	started := make(chan struct{}, 16)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if stall.Load() && r.Header.Get("Range") != "" && r.Header.Get("Range") != "bytes=0-0" {
			var start, end int
			if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
				t.Error(err)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
			w.WriteHeader(206)
			_, _ = w.Write(data[start : start+32768])
			w.(http.Flusher).Flush()
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(data))
	}))
	defer s.Close()
	dir := t.TempDir()
	m, err := Open(dir, false, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.Create(context.Background(), Options{URL: s.URL, Destination: filepath.Join(t.TempDir(), "result")})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := func() *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorker$")
		cmd.Env = append(os.Environ(), "IDM_TEST_STATE="+dir, "IDM_TEST_JOB="+j.ID, "IDM_TEST_STEP=")
		return cmd
	}
	cmd := command()
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("workers did not start")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	stall.Store(false)
	if output, err := command().CombinedOutput(); err != nil {
		t.Fatalf("relaunch: %v %s", err, output)
	}
	actual, err := os.ReadFile(j.Destination)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("active worker recovery mismatch", err)
	}
}
