//go:build integration

package integration

import (
	"bytes"
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNativeHostProcess(t *testing.T) {
	if runtime.GOOS == "linux" {
		d, e := os.MkdirTemp("", "idm-host-")
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = os.RemoveAll(d) }()
		t.Setenv("XDG_RUNTIME_DIR", d)
	}

	name := filepath.Join(t.TempDir(), "native-host")
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", name, "../cmd/native-host")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", b, e)
	}
	for _, mode := range []string{"absent", "partial", "oversize", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, name)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if mode == "timeout" {
				stdin, e := cmd.StdinPipe()
				if e != nil {
					t.Fatal(e)
				}
				defer func() { _ = stdin.Close() }()
				start := time.Now()
				e = cmd.Run()
				if e == nil || time.Since(start) < 4*time.Second || !bytes.Contains(stderr.Bytes(), []byte("timed out")) {
					t.Fatalf("timeout: %v %s", e, &stderr)
				}
				return
			}
			var input bytes.Buffer
			switch mode {
			case "absent":
				if e := protocol.Write(&input, protocol.Message{Version: 1, ID: "process", Type: "ping"}); e != nil {
					t.Fatal(e)
				}
			case "partial":
				input.Write([]byte{1, 0})
			case "oversize":
				input.Write([]byte{1, 0, 1, 0})
			}
			cmd.Stdin = &input
			e := cmd.Run()
			if mode == "absent" {
				if e != nil {
					t.Fatal(e)
				}
				m, e := protocol.Read(&stdout)
				if e != nil || m.Type != "error" || m.ID != "process" {
					t.Fatalf("response: %+v %v", m, e)
				}
			} else if e == nil || stdout.Len() != 0 {
				t.Fatal("invalid frame accepted or stdout polluted")
			}
		})
	}
}
