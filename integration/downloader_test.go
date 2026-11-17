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
		cmd := exec.Command(name, "-o", dest, "-sha256", checksum, server.URL)
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
		cmd := exec.CommandContext(ctx, name, "-o", filepath.Join(dir, "download"), server.URL+"/stall")
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
		if err != nil || len(entries) != 0 {
			t.Fatal("interrupt left partial/output")
		}
	})
}
