package desktop

import (
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"sync"
	"testing"
)

func TestService(t *testing.T) {
	s := New("ready")
	if s.Info().LastPing != "" || s.Info().IPC != "ready" || s.Info().Build == "" || s.Info().Platform == "" || s.Hello() == "" {
		t.Fatal("invalid initial state")
	}
	if _, e := s.Accept(protocol.Message{}); e == nil {
		t.Fatal("invalid request accepted")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			m := protocol.Message{Version: protocol.Version, ID: "concurrent", Type: "ping"}
			ack, e := s.Accept(m)
			if e != nil || protocol.Match(m, ack) != nil {
				t.Error("request failed")
			}
			_ = s.Info()
		})
	}
	wg.Wait()
	if s.Info().LastPing != "concurrent" {
		t.Fatal("last ping not published")
	}
}

func TestDisconnectedBridge(t *testing.T) {
	s := New("ready")
	s.Disconnect()
	if s.Info().IPC != "Browser bridge disconnected. Restart the application." {
		t.Fatal("bridge failure not presented")
	}
}

func TestTrayFallback(t *testing.T) {
	s := New("ready")
	s.SetTrayAvailable(false)
	if s.Info().Lifecycle != "No system tray host is available. Closing this window minimizes it to the taskbar; downloads keep running." {
		t.Fatal(s.Info())
	}
	s.SetTrayAvailable(true)
	if s.Info().Lifecycle != "Closing this window keeps downloads running in the system tray." {
		t.Fatal(s.Info())
	}
}
