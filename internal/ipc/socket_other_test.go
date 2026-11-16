//go:build !linux

package ipc

import (
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"strings"
	"testing"
)

// Unsupported platforms must fail explicitly, without attempting another IPC
// mechanism or launching an application. Shared framing is tested separately.
func TestDeferredPlatform(t *testing.T) {
	if _, err := SocketPath(); err == nil || !strings.Contains(err.Error(), "M5") {
		t.Fatalf("unexpected socket availability: %v", err)
	}
	if listener, err := Listen(); err == nil {
		_ = listener.Close()
		t.Fatal("live IPC must remain explicitly unsupported")
	}
	if _, err := Exchange(context.Background(), protocol.Message{Version: protocol.Version, ID: "deferred", Type: "ping"}); err == nil {
		t.Fatal("unsupported exchange succeeded")
	}
}
