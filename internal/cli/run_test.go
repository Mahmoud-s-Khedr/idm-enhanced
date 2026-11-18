package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/unknown" {
			w.(http.Flusher).Flush()
		}
		_, _ = io.WriteString(w, "hello")
	}))
	defer server.Close()
	for _, tc := range []struct {
		name    string
		flags   []string
		path    string
		code    int
		cancel  bool
		message string
	}{
		{name: "success", message: "Download complete", flags: []string{"-sha256", fmt.Sprintf("%x", sha256.Sum256([]byte("hello")))}},
		{name: "quiet", flags: []string{"-quiet"}, message: "Download complete"},
		{name: "unknown", path: "/unknown", message: "total unknown"},
		{name: "checksum", flags: []string{"-sha256", strings.Repeat("0", 64)}, code: 1, message: "SHA-256 mismatch"},
		{name: "cancel", cancel: true, code: 130, message: "cancelled"},
		{name: "timeout", path: "/stall", flags: []string{"-timeout", "50ms"}, code: 1, message: "timed out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "result")
			args := append([]string{"--state-dir", t.TempDir(), "-o", dest}, tc.flags...)
			args = append(args, server.URL+tc.path+"?token=secret")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			var output bytes.Buffer
			if code := Run(ctx, args, &output); code != tc.code {
				t.Fatalf("code %d: %s", code, &output)
			}
			if !strings.Contains(output.String(), tc.message) || strings.Contains(output.String(), "secret") {
				t.Fatalf("output: %s", &output)
			}
			if tc.name == "quiet" && strings.Contains(output.String(), "Progress") {
				t.Fatal("quiet printed progress")
			}
			if tc.code == 0 {
				data, err := os.ReadFile(dest)
				if err != nil || string(data) != "hello" {
					t.Fatalf("output file: %q %v", data, err)
				}
			} else if _, err := os.Lstat(dest); !os.IsNotExist(err) {
				t.Fatal("failed command published destination")
			}
		})
	}
}

func TestRunUsage(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"-h"}, 0}, {nil, 2}, {[]string{"-unknown=secret"}, 2},
		{[]string{"-o", "file", "-timeout", "secret"}, 2},
		{[]string{"-o", "file", "-timeout", "0", "http://example.invalid"}, 2},
		{[]string{"-o", "file", "http://example.invalid", "extra"}, 2},
	} {
		var output bytes.Buffer
		if code := Run(context.Background(), tc.args, &output); code != tc.code || !strings.Contains(output.String(), "Usage:") || strings.Contains(output.String(), "secret") {
			t.Fatalf("usage: %d %s", code, &output)
		}
	}
}
func TestPersistentCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer server.Close()
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "result")
	ctx := context.Background()
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := Run(ctx, append([]string{"--state-dir", dir}, args...), &out)
		return code, out.String()
	}
	if code, out := run("-quiet", "-o", dest, server.URL); code != 0 {
		t.Fatal(code, out)
	}
	code, out := run("status")
	if code != 0 || !strings.Contains(out, "completed") {
		t.Fatal(code, out)
	}
	id := strings.Fields(out)[0]
	for _, args := range [][]string{{"status", id}, {"resume", id}, {"restart", id}, {"cancel", "--keep-partial", id}, {"cancel", "--discard-partial", id}} {
		if code, out = run(args...); code != 0 {
			t.Fatal(code, out)
		}
	}
	for _, args := range [][]string{{"cancel", id}, {"cancel", "--keep-partial", "--discard-partial", id}, {"status", "missing"}, {"resume", "missing"}, {"status", "one", "two"}, {"resume"}} {
		if code, _ = run(args...); code == 0 {
			t.Fatal("accepted invalid command", args)
		}
	}
	if code, _ = run("cancel", "--keep-partial", "missing"); code != 1 {
		t.Fatal(code)
	}
	if code, _ = run("-sha256", "bad", "-o", filepath.Join(t.TempDir(), "file"), server.URL); code != 1 {
		t.Fatal(code)
	}
	var missing bytes.Buffer
	if code := Run(ctx, []string{"--state-dir", filepath.Join(t.TempDir(), "missing"), "status"}, &missing); code != 1 {
		t.Fatal(code)
	}
	if code := Run(ctx, []string{"--state-dir"}, &missing); code != 2 {
		t.Fatal(code)
	}
	// Status does not take the modifier lock.
	m, err := manager.Open(dir, false, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if code, out = run("status"); code != 0 {
		t.Fatal(code, out)
	}
	if code, out = run("-o", filepath.Join(t.TempDir(), "file"), server.URL); code != 1 || !strings.Contains(out, "another modifying") {
		t.Fatal(code, out)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueCommands(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "hello") }))
	defer s.Close()
	dir := t.TempDir()
	ctx := context.Background()
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := Run(ctx, append([]string{"--state-dir", dir}, args...), &out)
		return code, out.String()
	}
	ids := make([]string, 0, 2)
	destinations := make([]string, 0, 2)
	for range 2 {
		dest := filepath.Join(t.TempDir(), "result")
		destinations = append(destinations, dest)
		code, out := run("enqueue", "-o", dest, s.URL)
		if code != 0 {
			t.Fatal(code, out)
		}
		ids = append(ids, strings.Fields(out)[1])
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("enqueue downloaded")
		}
	}
	if code, out := run("reorder", ids[1], ids[0]); code != 0 {
		t.Fatal(code, out)
	}
	if code, out := run("run-queue", "-jobs", "2", "-per-host", "1", "-workers", "1", "-rate", "65536", "-retries", "0"); code != 0 {
		t.Fatal(code, out)
	}
	for _, dest := range destinations {
		b, err := os.ReadFile(dest)
		if err != nil || string(b) != "hello" {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"run-queue", "extra"}, {"enqueue"}, {"run-queue", "-workers", "0"}, {"reorder", "missing"}} {
		if code, _ := run(args...); code == 0 {
			t.Fatal("accepted invalid command", args)
		}
	}
}
