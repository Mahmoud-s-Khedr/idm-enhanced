package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/localfs"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var ErrDependency = errors.New("install FFmpeg and ffprobe to assemble media")
var ErrAssembly = errors.New("media assembly or output validation failed")

type cappedWriter struct {
	writer    io.Writer
	remaining int64
	cancel    context.CancelFunc
	exceeded  bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		w.exceeded = true
		if w.cancel != nil {
			w.cancel()
		}
		return 0, ErrLimit
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

// DownloadObject uses the ordinary engine transport, including its redirect,
// representation, length, limiter and cancellation checks. Objects are bounded
// complete transfers, never file-range checkpoint jobs.
func DownloadObject(ctx context.Context, client *http.Client, source string, dst io.Writer, inspect func(engine.Representation) error) (engine.Representation, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	writer := cappedWriter{writer: dst, remaining: MaxObjectBytes, cancel: cancel}
	return engine.Stream(ctx, client, engine.Request{Source: source}, &writer, nil, func(rep engine.Representation) error {
		if rep.Length > MaxObjectBytes {
			return ErrLimit
		}
		if inspect != nil {
			return inspect(rep)
		}
		return nil
	})
}

type tools struct{ ffmpeg, ffprobe string }

// Assemble reads only verified local MPEG-TS bytes supplied by the manager.
// No manifest, source URL or media-object filename reaches FFmpeg. Output is
// stream-copy fragmented MP4; no automatic transcoding or network protocols.
func Assemble(ctx context.Context, input io.Reader, inputBytes int64, output *os.File) error {
	return assemble(ctx, input, inputBytes, output, tools{"ffmpeg", "ffprobe"})
}
func assemble(ctx context.Context, input io.Reader, inputBytes int64, output *os.File, binaries tools) error {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "pipe", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v?", "-map", "0:a?", "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1"}
	return runAssembly(ctx, input, inputBytes, output, binaries, args, nil)
}

func runAssembly(ctx context.Context, input io.Reader, inputBytes int64, output *os.File, binaries tools, args []string, expected []string) error {
	ffmpeg, err := exec.LookPath(binaries.ffmpeg)
	if err != nil {
		return ErrDependency
	}
	ffprobe, err := exec.LookPath(binaries.ffprobe)
	if err != nil {
		return ErrDependency
	}
	if (input == nil && len(expected) == 0) || output == nil || inputBytes <= 0 || inputBytes > MaxObjectBytes*MaxObjects {
		return ErrAssembly
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err = output.Truncate(0); err != nil {
		return err
	}
	if _, err = output.Seek(0, io.SeekStart); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, ffmpeg, args...)
	command.Stdin = input
	writer := cappedWriter{writer: output, remaining: inputBytes*2 + (16 << 20), cancel: cancel}
	command.Stdout = &writer
	command.Stderr = io.Discard
	command.WaitDelay = 2 * time.Second
	if err = command.Run(); err != nil {
		if writer.exceeded {
			return ErrLimit
		}
		return errors.Join(ErrAssembly, err, ctx.Err())
	}
	if err = output.Sync(); err != nil {
		return err
	}
	var magic [4]byte
	if _, err = output.ReadAt(magic[:], 4); err != nil || string(magic[:]) != "ftyp" {
		return ErrAssembly
	}
	data, err := probeMP4(ctx, ffprobe, output.Name())
	if err != nil {
		return err
	}
	return validateTracks(data, expected)
}
func probeMP4(ctx context.Context, ffprobe, path string) ([]byte, error) {
	probeCtx, probeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer probeCancel()
	var data bytes.Buffer
	limited := cappedWriter{writer: &data, remaining: 65536, cancel: probeCancel}
	probe := exec.CommandContext(probeCtx, ffprobe, "-v", "error", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-show_entries", "stream=codec_type,codec_name:format=duration,size", "-of", "json", path)
	probe.Stdout = &limited
	probe.Stderr = io.Discard
	probe.WaitDelay = 2 * time.Second
	if err := probe.Run(); err != nil {
		if limited.exceeded {
			return nil, ErrLimit
		}
		return nil, errors.Join(ErrAssembly, err, probeCtx.Err())
	}
	return data.Bytes(), nil
}
func validateTracks(data []byte, expected []string) error {
	if err := validateOutput(data); err != nil {
		return err
	}
	if len(expected) > 0 {
		var result struct {
			Streams []struct {
				Type string `json:"codec_type"`
			} `json:"streams"`
		}
		if json.Unmarshal(data, &result) != nil || len(result.Streams) != len(expected) {
			return ErrAssembly
		}
		for i, kind := range expected {
			if result.Streams[i].Type != kind {
				return ErrAssembly
			}
		}
	}
	return nil
}
func validateOutput(data []byte) error {
	var result struct {
		Streams []struct {
			Type  string `json:"codec_type"`
			Codec string `json:"codec_name"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
		} `json:"format"`
	}
	if json.Unmarshal(data, &result) != nil || len(result.Streams) == 0 || len(result.Streams) > 2 {
		return ErrAssembly
	}
	duration, e1 := strconv.ParseFloat(result.Format.Duration, 64)
	size, e2 := strconv.ParseInt(result.Format.Size, 10, 64)
	if e1 != nil || e2 != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 86400 || size < 32 {
		return ErrAssembly
	}
	seen := map[string]bool{}
	for _, stream := range result.Streams {
		supported := (stream.Type == "video" && stream.Codec == "h264") || (stream.Type == "audio" && stream.Codec == "aac")
		if seen[stream.Type] || !supported {
			return ErrUnsupported
		}
		seen[stream.Type] = true
	}
	return nil
}

// TrackInput supplies already-owned, hash-verified initialization and fragments.
// Seekable private staging files support MOV demuxing on every native platform;
// no source URL, remote manifest or cache filename reaches the subprocess.
type TrackInput struct {
	Reader io.Reader
	Bytes  int64
	Kind   string
}

func AssembleTracks(ctx context.Context, tracks []TrackInput, output *os.File) error {
	return assembleTracks(ctx, tracks, output, tools{"ffmpeg", "ffprobe"})
}
func assembleTracks(ctx context.Context, tracks []TrackInput, output *os.File, binaries tools) (err error) {
	if output == nil || len(tracks) == 0 || len(tracks) > 2 {
		return ErrAssembly
	}
	if _, e := exec.LookPath(binaries.ffmpeg); e != nil {
		return ErrDependency
	}
	ffprobe, err := exec.LookPath(binaries.ffprobe)
	if err != nil {
		return ErrDependency
	}
	directory, err := os.MkdirTemp(filepath.Dir(output.Name()), ".idm-mux-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(directory)) }()
	if err = localfs.Protect(directory, true); err != nil {
		return err
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-max_alloc", "67108864", "-threads", "1"}
	var total int64
	var expected []string
	seen := map[string]bool{}
	for i, track := range tracks {
		if track.Reader == nil || track.Bytes <= 0 || track.Bytes > MaxObjectBytes*MaxObjects || (track.Kind != "audio" && track.Kind != "video") || seen[track.Kind] {
			return ErrAssembly
		}
		seen[track.Kind] = true
		path := filepath.Join(directory, strconv.Itoa(i)+".mp4")
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		n, e := io.Copy(f, io.LimitReader(track.Reader, track.Bytes+1))
		e = errors.Join(e, f.Close())
		if e != nil {
			return e
		}
		if n != track.Bytes {
			return ErrAssembly
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		data, e := probeMP4(ctx, ffprobe, path)
		if e != nil {
			return e
		}
		if e = validateTracks(data, []string{track.Kind}); e != nil {
			return e
		}
		args = append(args, "-protocol_whitelist", "file", "-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-i", path)
		total += track.Bytes
		expected = append(expected, track.Kind)
	}
	for i, track := range tracks {
		selector := "v:0"
		if track.Kind == "audio" {
			selector = "a:0"
		}
		args = append(args, "-map", strconv.Itoa(i)+":"+selector)
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")
	return runAssembly(ctx, nil, total, output, binaries, args, expected)
}
