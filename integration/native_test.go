//go:build integration

package integration

import (
	"bytes"
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/desktop"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/manager"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
	"net/http"
	"net/http/httptest"
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

func TestNativeHostDownloadHandoff(t *testing.T) {
	base := ""
	if runtime.GOOS != "windows" {
		base = "/tmp"
	}
	temporary, err := os.MkdirTemp(base, "idm-h-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	t.Setenv("XDG_RUNTIME_DIR", temporary)
	t.Setenv("HOME", temporary)
	t.Setenv("USERPROFILE", temporary)
	host := filepath.Join(temporary, "native-host")
	if runtime.GOOS == "windows" {
		host += ".exe"
	}
	if b, err := exec.Command("go", "build", "-o", host, "../cmd/native-host").CombinedOutput(); err != nil {
		t.Fatal(string(b), err)
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("M5 verified bytes")) }))
	defer fixture.Close()
	m, err := manager.Open(filepath.Join(temporary, "state"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.NewSession(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	service := desktop.New("test")
	service.Attach(session, desktop.PlatformActions{})
	l, err := ipc.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ipc.Serve(ctx, l, service.Accept) }()
	request := protocol.Message{Version: 1, ID: "native-handoff", Type: "download", URL: fixture.URL + "/file", Filename: "fixture.bin"}
	jobID := ""
	for range 2 {
		var input, output, diagnostics bytes.Buffer
		if err := protocol.Write(&input, request); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, host)
		cmd.Stdin = &input
		cmd.Stdout = &output
		cmd.Stderr = &diagnostics
		if err := cmd.Run(); err != nil {
			t.Fatal(err, diagnostics.String())
		}
		reply, err := protocol.Read(&output)
		if err != nil || reply.Type != "accepted" {
			t.Fatal(reply, err, diagnostics.String())
		}
		if jobID != "" && reply.JobID != jobID {
			t.Fatal("duplicate native handoff")
		}
		jobID = reply.JobID
	}
	jobs, err := session.Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].State != "queued" {
		t.Fatal(jobs, err)
	}
	if err := session.Start("", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		running, _, message := session.State()
		if !running {
			if message != "" {
				t.Fatal(message)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("download timed out")
		}
		time.Sleep(time.Millisecond)
	}
	output, err := os.ReadFile(filepath.Join(temporary, "Downloads", "fixture.bin"))
	if err != nil || string(output) != "M5 verified bytes" {
		t.Fatal(string(output), err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
