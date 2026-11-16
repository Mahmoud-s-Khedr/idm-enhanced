package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

func privateRuntime(t *testing.T) {
	t.Helper()
	d, e := os.MkdirTemp("", "idm-ipc-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	t.Setenv("XDG_RUNTIME_DIR", d)
}
func TestRoundTrip(t *testing.T) {
	privateRuntime(t)
	l, e := Listen()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, protocol.Ack) }()
	m := protocol.Message{Version: 1, ID: "integration", Type: "ping"}
	reply, e := Exchange(ctx, m)
	if e != nil || protocol.Match(m, reply) != nil {
		t.Fatalf("%+v %v", reply, e)
	}
	path, e := SocketPath()
	if e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permission: %v", e)
	}
	if other, e := Listen(); e == nil {
		_ = other.Close()
		t.Fatal("second listener accepted")
	}
	cancel()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
func TestAbsentAndInvalid(t *testing.T) {
	privateRuntime(t)
	if _, e := Exchange(context.Background(), protocol.Message{}); e == nil {
		t.Fatal("invalid request accepted")
	}
	m := protocol.Message{Version: 1, ID: "x", Type: "ping"}
	if _, e := Exchange(context.Background(), m); e == nil || !strings.Contains(e.Error(), "open IDM Enhanced") {
		t.Fatalf("absent error %v", e)
	}
}
func TestPrivateDirectory(t *testing.T) {
	privateRuntime(t)
	path, e := SocketPath()
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Dir(path)
	if e = os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = SocketPath(); e == nil {
		t.Fatal("accepted public directory")
	}
	if e = os.Remove(dir); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(t.TempDir(), dir); e != nil {
		t.Fatal(e)
	}
	if _, e = SocketPath(); e == nil {
		t.Fatal("accepted symlink")
	}
}
func TestFallbackAndBadBase(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, e := SocketPath(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "absent", "nested"))
	if _, e := SocketPath(); e == nil {
		t.Fatal("accepted missing parent")
	}
	d := filepath.Join(t.TempDir(), strings.Repeat("x", 100))
	if e := os.Mkdir(d, 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("XDG_RUNTIME_DIR", d)
	if _, e := SocketPath(); e == nil {
		t.Fatal("accepted long path")
	}
}
func TestCancellationAndBadReply(t *testing.T) {
	for _, mode := range []string{"timeout", "bad-reply", "handler-failure"} {
		t.Run(mode, func(t *testing.T) {
			privateRuntime(t)
			l, e := Listen()
			if e != nil {
				t.Fatal(e)
			}
			server, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- Serve(server, l, func(m protocol.Message) (protocol.Message, error) {
					if mode == "timeout" {
						<-server.Done()
						return protocol.Message{}, server.Err()
					}
					if mode == "handler-failure" {
						return protocol.Message{}, errors.New("failed")
					}
					m.Type = "ack"
					m.ID = "different"
					return m, nil
				})
			}()
			ctx, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer stop()
			if _, e = Exchange(ctx, protocol.Message{Version: 1, ID: "x", Type: "ping"}); e == nil {
				t.Fatal("invalid response succeeded")
			}
			cancel()
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestMalformedConnectionAndAcceptFailure(t *testing.T) {
	privateRuntime(t)
	l, e := Listen()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, protocol.Ack) }()
	path, e := SocketPath()
	if e != nil {
		t.Fatal(e)
	}
	c, e := net.Dial("unix", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Write([]byte{0, 0, 0, 0}); e != nil {
		t.Fatal(e)
	}
	_ = c.Close()
	cancel()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	l, e = Listen()
	if e != nil {
		t.Fatal(e)
	}
	_ = l.Close()
	if e = Serve(context.Background(), l, protocol.Ack); e == nil {
		t.Fatal("swallowed accept failure")
	}
}
