package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const probeGood = `{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}],"format":{"duration":"1.0","size":"64"}}`

func TestMain(m *testing.M) {
	if os.Getenv("IDM_MEDIA_TOOL_HELPER") == "1" {
		mode := os.Getenv("IDM_MEDIA_TOOL_MODE")
		name := filepath.Base(os.Args[0])
		args := strings.Join(os.Args[1:], " ")
		if strings.Contains(name, "ffmpeg") {
			if !strings.Contains(args, "-protocol_whitelist pipe -f mpegts -i pipe:0") || !strings.Contains(args, "-c copy") || strings.Contains(args, "http") {
				os.Exit(9)
			}
			if mode == "hang" {
				for {
					time.Sleep(time.Second)
				}
			}
			if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
				os.Exit(8)
			}
			if mode == "ffmpeg_error" {
				os.Exit(3)
			}
			data := append([]byte{0, 0, 0, 24}, []byte("ftypisom0000000000000000000000000000000000000000000000000000")...)
			if mode == "bad_magic" {
				data = []byte("not mp4")
			}
			if mode == "empty_output" {
				data = nil
			}
			if mode == "oversized" {
				data = bytes.Repeat([]byte("x"), 17<<20)
			}
			if _, err := os.Stdout.Write(data); err != nil {
				os.Exit(7)
			}
		} else {
			if !strings.Contains(args, "-protocol_whitelist file -f mov") {
				os.Exit(9)
			}
			if mode == "probe_error" {
				os.Exit(4)
			}
			data := probeGood
			if mode == "bad_json" {
				data = "{"
			}
			if mode == "bad_codec" {
				data = strings.ReplaceAll(data, "h264", "hevc")
			}
			if mode == "large_probe" {
				data = strings.Repeat("x", 70000)
			}
			if _, err := fmt.Fprint(os.Stdout, data); err != nil {
				os.Exit(7)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeTools(t *testing.T) tools {
	t.Helper()
	t.Setenv("IDM_MEDIA_TOOL_HELPER", "1")
	directory := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	makeTool := func(name string) string {
		path := filepath.Join(directory, name+suffix)
		src, err := os.Open(os.Args[0])
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(dst, src)
		err = errors.Join(err, dst.Close(), src.Close())
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	return tools{makeTool("fake-ffmpeg"), makeTool("fake-ffprobe")}
}
func TestAssemblyOfflineProtocolsValidationAndFailureRetention(t *testing.T) {
	binaries := fakeTools(t)
	for _, mode := range []string{"ok", "ffmpeg_error", "bad_magic", "empty_output", "probe_error", "bad_json", "bad_codec", "oversized", "large_probe", "hang"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("IDM_MEDIA_TOOL_MODE", mode)
			output, err := os.CreateTemp(t.TempDir(), "output-*.part")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := output.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx := context.Background()
			if mode == "hang" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			err = assemble(ctx, strings.NewReader("verified TS objects"), 19, output, binaries)
			if (err == nil) != (mode == "ok") {
				t.Fatalf("%s: %v", mode, err)
			}
			if mode == "hang" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("lost cancellation", err)
			}
			if _, statErr := os.Stat(output.Name()); statErr != nil {
				t.Fatal("assembly removed owned output", statErr)
			}
		})
	}
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	if err = assemble(context.Background(), strings.NewReader("x"), 1, output, tools{"missing-ffmpeg-unique", binaries.ffprobe}); !errors.Is(err, ErrDependency) {
		t.Fatal(err)
	}
	if err = assemble(context.Background(), strings.NewReader("x"), 1, output, tools{binaries.ffmpeg, "missing-ffprobe-unique"}); !errors.Is(err, ErrDependency) {
		t.Fatal(err)
	}
	if err = assemble(context.Background(), nil, 1, output, binaries); !errors.Is(err, ErrAssembly) {
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	if err = assemble(context.Background(), strings.NewReader("x"), 1, output, binaries); err == nil {
		t.Fatal("accepted closed destination")
	}
}
func TestOutputRejectsUnsupportedOrEmptyMedia(t *testing.T) {
	for _, value := range []string{"{}", "{", strings.Replace(probeGood, "1.0", "NaN", 1), strings.Replace(probeGood, "1.0", "Inf", 1), strings.Replace(probeGood, "1.0", "0", 1), strings.Replace(probeGood, "64", "bad", 1), strings.Replace(probeGood, "64", "0", 1), strings.Replace(probeGood, "h264", "hevc", 1), strings.Replace(probeGood, "audio", "video", 1), strings.Replace(probeGood, "aac", "unknown", 1)} {
		if err := validateOutput([]byte(value)); err == nil {
			t.Fatal("accepted invalid output", value)
		}
	}
	if err := validateOutput([]byte(probeGood)); err != nil {
		t.Fatal(err)
	}
}
func TestObjectSizeBoundAndWriterErrors(t *testing.T) {
	var out bytes.Buffer
	cancelled := false
	writer := cappedWriter{writer: &out, remaining: 2, cancel: func() { cancelled = true }}
	if n, err := writer.Write([]byte("ok")); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if _, err := writer.Write([]byte("x")); !errors.Is(err, ErrLimit) || !cancelled || out.String() != "ok" {
		t.Fatal(err)
	}
	writer = cappedWriter{writer: &out, remaining: 0}
	if _, err := writer.Write([]byte("x")); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
