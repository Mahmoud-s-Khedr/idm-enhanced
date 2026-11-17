package manager

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// This helper is only compiled into the test binary. Production has no fault env vars.
func TestCrashWorker(t *testing.T) {
	dir := os.Getenv("IDM_TEST_STATE")
	if dir == "" {
		return
	}
	m, err := Open(dir, false, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	step := os.Getenv("IDM_TEST_STEP")
	if step != "" {
		m.boundary = func(name string) error {
			if name == step {
				fmt.Println("barrier")
				select {}
			}
			return nil
		}
	}
	err = m.Run(context.Background(), os.Getenv("IDM_TEST_JOB"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestProcessCrashRecovery(t *testing.T) {
	for _, step := range []string{"before-file-sync", "after-file-sync", "after-checkpoint", "before-verified-commit", "after-verified-commit", "after-publish", "after-remove", "after-completed"} {
		t.Run(step, func(t *testing.T) {
			_, source := fixture(t)
			dir := t.TempDir()
			m, err := Open(dir, false, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "result")
			j, err := m.Create(context.Background(), Options{URL: source, Destination: dest})
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := func(boundary string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorker$")
				cmd.Env = append(os.Environ(), "IDM_TEST_STATE="+dir, "IDM_TEST_JOB="+j.ID, "IDM_TEST_STEP="+boundary)
				return cmd
			}
			cmd := command(step)
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(pipe)
			if !scanner.Scan() || scanner.Text() != "barrier" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("worker did not reach crash boundary", scanner.Err())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			// Relaunch a fresh process; the OS must have released the owner lock.
			if output, err := command("").CombinedOutput(); err != nil {
				t.Fatalf("relaunch: %v %s", err, output)
			}
			data, err := os.ReadFile(dest)
			if err != nil || string(data) != "hello" {
				t.Fatal("incorrect crash recovery", string(data), err)
			}
			ro, err := Open(dir, true, http.DefaultClient)
			if err != nil {
				t.Fatal(err)
			}
			jobs, err := ro.Status(context.Background())
			if err != nil || jobs[0].State != "completed" {
				t.Fatal(jobs, err)
			}
			if err = ro.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
