package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransfer(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/error":
			http.Error(w, "no", http.StatusNotFound)
		default:
			_, _ = w.Write([]byte("hello"))
		}
	}))
	defer s.Close()
	var out bytes.Buffer
	var got int64
	if err := Transfer(context.Background(), s.Client(), s.URL+"/redirect", &out, func(n, _ int64) { got = n }); err != nil || out.String() != "hello" || got != 5 {
		t.Fatalf("transfer: %q %d %v", out.String(), got, err)
	}
	if err := Transfer(context.Background(), s.Client(), s.URL+"/error", io.Discard, nil); err == nil {
		t.Fatal("expected status error")
	}
	if err := Transfer(context.Background(), s.Client(), "file:///tmp/a", io.Discard, nil); err == nil {
		t.Fatal("expected URL error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Transfer(ctx, s.Client(), s.URL, io.Discard, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
