package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResumeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, etag, encoding, cr, body string
		status                         int
		length                         int64
		unsafe                         bool
	}{
		{"valid", `"v1"`, "", "bytes 2-4/5", "llo", 206, 3, false},
		{"ignored", `"v1"`, "", "", "hello", 200, 5, true},
		{"changed", `"v2"`, "", "bytes 2-4/5", "llo", 206, 3, true},
		{"encoding", `"v1"`, "gzip", "bytes 2-4/5", "llo", 206, 3, true},
		{"range missing", `"v1"`, "", "", "llo", 206, 3, true},
		{"start", `"v1"`, "", "bytes 1-4/5", "ello", 206, 4, true},
		{"total", `"v1"`, "", "bytes 2-5/6", "lloo", 206, 4, true},
		{"partial", `"v1"`, "", "bytes 2-3/5", "ll", 206, 2, true},
		{"short", `"v1"`, "", "bytes 2-4/5", "ll", 206, -1, true},
		{"long", `"v1"`, "", "bytes 2-4/5", "lloo", 206, -1, true},
		{"bad length", `"v1"`, "", "bytes 2-4/5", "llo", 206, 4, true},
		{"temporary server failure", `"v1"`, "", "", "", 503, 0, true},
		{"416", `"v1"`, "", "bytes */5", "", 416, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Range") != "bytes=2-" || r.Header.Get("If-Range") != `"v1"` {
					t.Error("missing resume conditions")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Etag": {tc.etag}, "Content-Range": {tc.cr}, "Content-Encoding": {tc.encoding}}, ContentLength: tc.length, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			var out bytes.Buffer
			_, err := Stream(context.Background(), client, Request{Source: "https://example.invalid", Resume: true, Offset: 2, Identity: Representation{Effective: "https://example.invalid", ETag: `"v1"`, Length: 5}}, &out, nil, nil)
			if (err != nil) != tc.unsafe {
				t.Fatalf("got %v", err)
			}
			if !tc.unsafe && out.String() != "llo" {
				t.Fatal(out.String())
			}
		})
	}
}
func TestProbe(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=0-0" {
			t.Error("bad probe")
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.WriteHeader(206)
		_, _ = io.WriteString(w, "h")
	}))
	defer s.Close()
	rep, err := Stream(context.Background(), s.Client(), Request{Source: s.URL, Resume: true, Probe: true, Identity: Representation{Effective: s.URL, ETag: `"v1"`, Length: -1}}, io.Discard, nil, nil)
	if err != nil || rep.Length != 5 {
		t.Fatalf("%+v %v", rep, err)
	}
	for _, etag := range []string{"", `W/"v1"`, "bad", `"bad"tag"`, "\"bad\tvalue\"", "\"bad\x7fvalue\""} {
		if _, err := Stream(context.Background(), s.Client(), Request{Source: s.URL, Resume: true, Identity: Representation{ETag: etag}}, io.Discard, nil, nil); !errors.Is(err, ErrUnsafeResume) {
			t.Fatal(err)
		}
	}
	if _, err := Stream(context.Background(), s.Client(), Request{Source: s.URL, Resume: true, Probe: true, Identity: Representation{Effective: s.URL, ETag: `"v1"`, Length: 5}}, io.Discard, nil, func(Representation) error { return errors.New("persist failed") }); err == nil {
		t.Fatal("ignored inspect failure")
	}
}
func TestRangeSyntax(t *testing.T) {
	for _, value := range []string{"", "bytes 0-0", "bytes 0/1", "bytes x-0/1", "bytes 0-x/1", "bytes 0-0/x", "bytes -1-0/1", "bytes 2-1/3", "bytes 0-1/1", "bytes 0-0/1/2", "bytes +0-0/1", "bytes 0-+0/1", "bytes 0-0/+1"} {
		if _, _, _, ok := contentRange(value); ok {
			t.Fatal(value)
		}
	}
}
