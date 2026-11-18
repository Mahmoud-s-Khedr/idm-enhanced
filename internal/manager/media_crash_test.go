package manager

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMediaProcessCrashRecovery(t *testing.T) { testMediaProcessCrashRecovery(t, false) }
func TestDASHProcessCrashRecovery(t *testing.T)  { testMediaProcessCrashRecovery(t, true) }
func testMediaProcessCrashRecovery(t *testing.T, dash bool) {
	for _, boundary := range []string{"after-media-file-sync", "after-media-checkpoint", "before-media-assembly", "after-media-assembly", "before-verified-commit", "after-verified-commit", "after-publish", "after-remove", "after-completed"} {
		t.Run(boundary, func(t *testing.T) {
			m, server, _ := mediaFixture(t)
			if dash {
				var other *Manager
				other, server, _, _ = dashFixture(t)
				if err := m.Close(); err != nil {
					t.Fatal(err)
				}
				m = other
			}
			var j Job
			if dash {
				j = dashJob(t, m, server)
			} else {
				j = mediaJob(t, m, server)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := func(step string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorker$")
				cmd.Env = append(os.Environ(), "IDM_TEST_STATE="+m.directory, "IDM_TEST_JOB="+j.ID, "IDM_TEST_STEP="+step, "IDM_TEST_MEDIA=1")
				return cmd
			}
			cmd := command(boundary)
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
				t.Fatal("media worker did not reach boundary", scanner.Err())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			if output, err := command("").CombinedOutput(); err != nil {
				t.Fatalf("media relaunch failed: %v %s", err, output)
			}
			if dash {
				assertDASHOutput(t, j)
			} else {
				assertMediaOutput(t, j)
			}
			ro, err := Open(m.directory, true, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			jobs, err := ro.Status(ctx)
			if err != nil || len(jobs) != 1 || jobs[0].State != "completed" {
				t.Fatal(jobs, err)
			}
			if err = ro.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
