package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFixtures(t *testing.T) {
	block := bytes.Repeat([]byte("fixture"), 4096)
	hash := sha256.New()
	for range 64 {
		_, _ = hash.Write(block)
	}
	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			t.Error("HEAD must not be required")
			w.WriteHeader(405)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/known", http.StatusFound)
			return
		}
		if r.URL.Path == "/missing" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Path == "/short" {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("short"))
			return
		}
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("identity representation required")
		}
		if r.URL.Path == "/known" {
			w.Header().Set("Content-Length", fmt.Sprint(len(block)*64))
		} else {
			w.(http.Flusher).Flush()
		}
		for range 64 {
			if _, err := w.Write(block); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	for _, path := range []string{"/known", "/unknown", "/redirect", "/short", "/missing"} {
		t.Run(path, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "result")
			var total, written int64
			err := Download(context.Background(), server.Client(), Options{URL: server.URL + path, Destination: dest, SHA256: checksum, Progress: func(n, size int64) { written, total = n, size }})
			fail := path == "/short" || path == "/missing"
			if (err != nil) != fail {
				t.Fatalf("download error: %v", err)
			}
			entries, readErr := os.ReadDir(filepath.Dir(dest))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if fail {
				if len(entries) != 1 {
					t.Fatal("failure left output/partial")
				}
				return
			}
			data, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != checksum || written != int64(len(data)) {
				t.Fatal("incorrect bytes/progress")
			}
			if path == "/unknown" && total != -1 {
				t.Fatalf("unknown total: %d", total)
			}
			if len(entries) != 1 {
				t.Fatal("partial remains after success")
			}
		})
	}
}

func TestDownloadBoundaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer server.Close()
	for _, tc := range []struct {
		name, checksum   string
		existing, cancel bool
	}{
		{name: "checksum", checksum: strings.Repeat("0", 64)},
		{name: "invalid checksum", checksum: "bad"},
		{name: "existing", existing: true},
		{name: "cancelled", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "result")
			if tc.existing {
				if err := os.WriteFile(dest, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			err := Download(ctx, server.Client(), Options{URL: server.URL, Destination: dest, SHA256: tc.checksum})
			if err == nil {
				t.Fatal("expected failure")
			}
			entries, err := os.ReadDir(filepath.Dir(dest))
			if err != nil {
				t.Fatal(err)
			}
			if tc.existing {
				data, err := os.ReadFile(dest)
				if err != nil || string(data) != "original" {
					t.Fatal("overwrote destination")
				}
			} else if tc.name == "checksum" && len(entries) != 1 {
				t.Fatal("left partial/output")
			}
		})
	}
	if err := Download(context.Background(), server.Client(), Options{}); err == nil {
		t.Fatal("accepted empty destination")
	}
	if err := Download(context.Background(), server.Client(), Options{URL: server.URL, Destination: filepath.Join(t.TempDir(), "missing", "result")}); err == nil {
		t.Fatal("accepted missing directory")
	}
}

func TestDownloadPublishCollision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "result")
	created := false
	err := Download(context.Background(), server.Client(), Options{URL: server.URL, Destination: dest, Progress: func(_, _ int64) {
		if !created {
			created = true
			if err := os.WriteFile(dest, []byte("other"), 0600); err != nil {
				t.Error(err)
			}
		}
	}})
	if err == nil {
		t.Fatal("published over concurrent file")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "other" {
		t.Fatal("changed concurrent file")
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil || len(entries) != 2 {
		t.Fatal("partial cleanup failed")
	}
}

func TestDownloadSymlink(t *testing.T) {
	dir := t.TempDir()
	target, dest := filepath.Join(dir, "target"), filepath.Join(dir, "result")
	if err := os.WriteFile(target, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Download(context.Background(), http.DefaultClient, Options{URL: "http://unused.invalid", Destination: dest}); err == nil {
		t.Fatal("accepted symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "safe" {
		t.Fatal("modified symlink target")
	}
}

func Download(ctx context.Context, client *http.Client, o Options) error {
	dir, err := os.MkdirTemp("", "idm-manager-test-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	m, err := Open(dir, false, client)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	c := DefaultConfig()
	c.Retries = 0
	if err := m.Configure(c, client); err != nil {
		return err
	}
	j, err := m.Create(ctx, o)
	if err != nil {
		return err
	}
	return m.Run(ctx, j.ID, false, o.Progress)
}
