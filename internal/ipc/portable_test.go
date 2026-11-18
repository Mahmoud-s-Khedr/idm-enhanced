package ipc

import (
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"os"
	"runtime"
	"testing"
)

func TestNativePlatformExchange(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "idm-p-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, l, func(m protocol.Message) (protocol.Message, error) {
			return protocol.Message{Version: 1, ID: m.ID, Type: "accepted", JobID: "durable-job"}, nil
		})
	}()
	request := protocol.Message{Version: 1, ID: "native-platform", Type: "download", URL: "https://example.invalid/a", Filename: "a"}
	reply, err := Exchange(ctx, request)
	if err != nil || reply.JobID != "durable-job" {
		t.Fatal(reply, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
