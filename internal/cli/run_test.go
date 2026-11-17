package cli

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
			args := append([]string{"-o", dest}, tc.flags...)
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
