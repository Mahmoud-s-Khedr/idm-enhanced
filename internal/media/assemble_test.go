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

const probeGood = `{"streams":[{"codec_type":"video","codec_name":"h264","duration":"1.0","start_time":"0.0"},{"codec_type":"audio","codec_name":"aac","duration":"1.0","start_time":"0.0"}],"format":{"duration":"1.0","size":"64"}}`

func TestMain(m *testing.M) {
	if os.Getenv("IDM_MEDIA_TOOL_HELPER") == "1" {
		mode := os.Getenv("IDM_MEDIA_TOOL_MODE")
		name := filepath.Base(os.Args[0])
		args := strings.Join(os.Args[1:], " ")
		if strings.Contains(name, "ffmpeg") {
			hls := strings.Contains(args, "-protocol_whitelist pipe -f mpegts -i pipe:0")
			dash := strings.Contains(args, "-protocol_whitelist file -f mov -enable_drefs 0 -use_absolute_path 0 -i")
			if (!hls && !dash) || !strings.Contains(args, "-c copy") || strings.Contains(args, "http") {
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
			if dash {
				if !strings.Contains(args, "-use_editlist 1 -movie_timescale 1000000 -fs") || !strings.Contains(args, "-n -f mp4") {
					os.Exit(9)
				}
				if err := os.WriteFile(os.Args[len(os.Args)-1], data, 0600); err != nil {
					os.Exit(7)
				}
			} else if _, err := os.Stdout.Write(data); err != nil {
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
			if strings.Contains(args, ".idm-mux-") {
				codec, kind := "h264", "video"
				if strings.Contains(args, "1.mp4") {
					codec, kind = "aac", "audio"
				}
				data = fmt.Sprintf(`{"streams":[{"codec_type":%q,"codec_name":%q,"duration":"1.0","start_time":"0.0"}],"format":{"duration":"1.0","size":"64"}}`, kind, codec)
			}
			if mode == "stream_duration" {
				data = strings.Replace(data, `"duration":"1.0"`, `"duration":"0.5"`, 1)
			}
			if mode == "output_duration" && !strings.Contains(args, ".idm-mux-") {
				data = strings.ReplaceAll(data, "1.0", "1.5")
			}
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
	for _, value := range []string{"{}", "{", strings.Replace(probeGood, `"format":{"duration":"1.0"`, `"format":{"duration":"NaN"`, 1), strings.Replace(probeGood, `"format":{"duration":"1.0"`, `"format":{"duration":"Inf"`, 1), strings.Replace(probeGood, `"format":{"duration":"1.0"`, `"format":{"duration":"0"`, 1), strings.Replace(probeGood, "64", "bad", 1), strings.Replace(probeGood, "64", "0", 1), strings.Replace(probeGood, "h264", "hevc", 1), strings.Replace(probeGood, "audio", "video", 1), strings.Replace(probeGood, "aac", "unknown", 1)} {
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

func TestDASHMuxLocalFilesAndCleanup(t *testing.T) {
	binaries := fakeTools(t)
	for _, mode := range []string{"ok", "ffmpeg_error", "bad_magic", "bad_codec", "hang", "output_duration", "oversized", "stream_duration"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("IDM_MEDIA_TOOL_MODE", mode)
			directory := t.TempDir()
			out, err := os.CreateTemp(directory, "out-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = out.Close() }()
			ctx := context.Background()
			if mode == "hang" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			tracks := []TrackInput{{Reader: strings.NewReader("initvideo"), Bytes: 9, Kind: "video", Duration: 1}, {Reader: strings.NewReader("initaudio"), Bytes: 9, Kind: "audio", Duration: 1}}
			err = assembleTracks(ctx, tracks, out, binaries)
			if (err == nil) != (mode == "ok") {
				t.Fatal(mode, err)
			}
			entries, e := os.ReadDir(directory)
			if e != nil || len(entries) != 1 {
				t.Fatal("leaked staging files", entries, e)
			}
		})
	}
	t.Setenv("IDM_MEDIA_TOOL_MODE", "ok")
	out, err := os.CreateTemp(t.TempDir(), "out-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	for _, tracks := range [][]TrackInput{nil, {{Reader: nil, Bytes: 1, Kind: "video", Duration: 1}}, {{Reader: strings.NewReader("x"), Bytes: 1, Kind: "subtitle", Duration: 1}}, {{Reader: strings.NewReader("x"), Bytes: 2, Kind: "video", Duration: 1}}, {{Reader: strings.NewReader("xx"), Bytes: 1, Kind: "video", Duration: 1}}, {{Reader: strings.NewReader("x"), Bytes: 1, Kind: "video", Duration: 1}, {Reader: strings.NewReader("x"), Bytes: 1, Kind: "video", Duration: 1}}, {{Reader: strings.NewReader("x"), Bytes: 1, Kind: "audio", Duration: 1}}} {
		if err := assembleTracks(context.Background(), tracks, out, binaries); err == nil {
			t.Fatal("accepted invalid input/mismatched streams", tracks)
		}
	}
	if err := assembleTracks(context.Background(), nil, nil, binaries); err == nil {
		t.Fatal("nil output")
	}
}
