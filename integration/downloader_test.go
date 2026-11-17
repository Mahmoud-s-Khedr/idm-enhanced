//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDownloaderProcess(t *testing.T) {
	name := filepath.Join(t.TempDir(), "downloader")
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if b, err := exec.Command("go", "build", "-o", name, "../cmd/downloader").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", b, err)
	}
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			_, _ = io.WriteString(w, "partial")
			w.(http.Flusher).Flush()
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, "process fixture")
	}))
	defer server.Close()
	t.Run("verified file", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "download")
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte("process fixture")))
		cmd := exec.Command(name, "--state-dir", t.TempDir(), "-o", dest, "-sha256", checksum, server.URL)
		if b, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(b, []byte("Download complete")) {
			t.Fatalf("process: %s %v", b, err)
		}
		data, err := os.ReadFile(dest)
		if err != nil || string(data) != "process fixture" {
			t.Fatalf("file: %q %v", data, err)
		}
	})
	t.Run("interrupt", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("os.Interrupt delivery unsupported by Go on Windows; context cancellation tested in cli suite")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dir := t.TempDir()
		cmd := exec.CommandContext(ctx, name, "--state-dir", t.TempDir(), "-o", filepath.Join(dir, "download"), server.URL+"/stall")
		var output bytes.Buffer
		cmd.Stderr = &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-ctx.Done():
			_ = cmd.Wait()
			t.Fatal("server did not start")
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal(err)
		}
		err := cmd.Wait()
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 130 || !bytes.Contains(output.Bytes(), []byte("cancelled")) {
			t.Fatalf("interrupt: %v %s", err, &output)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatal("interrupt did not retain exactly one partial")
		}
	})
}

func TestDownloaderResumeProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go cannot deliver SIGINT on Windows; native process-kill recovery is covered by manager tests")
	}
	name := filepath.Join(t.TempDir(), "downloader")
	if output, err := exec.Command("go", "build", "-o", name, "../cmd/downloader").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	data := bytes.Repeat([]byte("resume fixture"), 32768)
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		value := r.Header.Get("Range")
		if value == "bytes=0-0" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(data)))
			w.WriteHeader(206)
			_, _ = w.Write(data[:1])
			return
		}
		if value != "" {
			var offset int
			if _, err := fmt.Sscanf(value, "bytes=%d-", &offset); err != nil || offset < 0 || offset >= len(data) {
				w.WriteHeader(416)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(data)-1, len(data)))
			w.WriteHeader(206)
			_, _ = w.Write(data[offset:])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		_, _ = w.Write(data[:32768])
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	state := t.TempDir()
	dest := filepath.Join(t.TempDir(), "result")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, "--state-dir", state, "-o", dest, server.URL+"?token=private")
	output := progressCapture{ready: make(chan struct{})}
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("download never started")
	}
	// Wait for client-observed progress, rather than only the server's write.
	// Otherwise cancellation can precede receipt of the resume validator.
	select {
	case <-output.ready:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("client did not observe response metadata")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	err := cmd.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 130 {
		t.Fatalf("pause: %v: %s", err, &output)
	}
	fields := strings.Fields(output.String())
	if len(fields) < 2 || fields[0] != "Job:" {
		t.Fatal(output.String())
	}
	id := fields[1]
	if strings.Contains(output.String(), "private") {
		t.Fatal("secret in output")
	}
	for _, args := range [][]string{{"status", id}, {"resume", "-quiet", id}, {"status", id}} {
		args = append([]string{"--state-dir", state}, args...)
		// Each independently launched command gets its own deadline. macOS
		// executable validation/startup must not consume later commands' budgets.
		commandCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		b, err := exec.CommandContext(commandCtx, name, args...).CombinedOutput()
		stop()
		if err != nil {
			t.Fatalf("command %v: %v: %s", args, err, b)
		}
		if bytes.Contains(b, []byte("private")) {
			t.Fatal("secret in diagnostics")
		}
	}
	got, err := os.ReadFile(dest)
	if err != nil || sha256.Sum256(got) != sha256.Sum256(data) {
		t.Fatal("incorrect resumed output", err)
	}
}

// exec.Wait joins the output copier before tests inspect Buffer.
type progressCapture struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (c *progressCapture) Write(p []byte) (int, error) {
	n, err := c.buffer.Write(p)
	if bytes.Contains(c.buffer.Bytes(), []byte("Progress:")) {
		c.once.Do(func() { close(c.ready) })
	}
	return n, err
}

func (c *progressCapture) String() string { return c.buffer.String() }
