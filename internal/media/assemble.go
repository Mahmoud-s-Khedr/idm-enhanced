package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var ErrDependency = errors.New("install FFmpeg and ffprobe to assemble HLS media")
var ErrAssembly = errors.New("HLS assembly or output validation failed")

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
	ffmpeg, err := exec.LookPath(binaries.ffmpeg)
	if err != nil {
		return ErrDependency
	}
	ffprobe, err := exec.LookPath(binaries.ffprobe)
	if err != nil {
		return ErrDependency
	}
	if input == nil || output == nil || inputBytes <= 0 || inputBytes > MaxObjectBytes*MaxObjects {
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
	command := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "pipe", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v?", "-map", "0:a?", "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy", "-bsf:a", "aac_adtstoasc", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")
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
	probeCtx, probeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer probeCancel()
	var data bytes.Buffer
	limited := cappedWriter{writer: &data, remaining: 65536, cancel: probeCancel}
	probe := exec.CommandContext(probeCtx, ffprobe, "-v", "error", "-max_alloc", "67108864", "-threads", "1", "-protocol_whitelist", "file", "-f", "mov", "-show_entries", "stream=codec_type,codec_name:format=duration,size", "-of", "json", output.Name())
	probe.Stdout = &limited
	probe.Stderr = io.Discard
	probe.WaitDelay = 2 * time.Second
	if err = probe.Run(); err != nil {
		if limited.exceeded {
			return ErrLimit
		}
		return errors.Join(ErrAssembly, err, probeCtx.Err())
	}
	return validateOutput(data.Bytes())
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
