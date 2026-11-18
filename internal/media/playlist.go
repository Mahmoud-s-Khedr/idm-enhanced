// Package media parses a deliberately bounded HLS VOD profile. It does not own
// job lifecycle, credentials, persistence, queues or file publication.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxPlaylist = 1 << 20
const MaxObjects = 2048
const MaxObjectBytes int64 = 256 << 20

var ErrUnsupported = errors.New("unsupported HLS VOD profile")
var ErrLimit = errors.New("HLS resource limit exceeded")

type Variant struct {
	ID         string `json:"id"`
	Bandwidth  int64  `json:"bandwidth"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
	URL        string `json:"url"`
}
type Playlist struct {
	Digest   string
	Variants []Variant
	Segments []string
	Duration float64
}
type Plan struct {
	SourceDigest   string
	PlaylistDigest string
	PlaylistURL    string
	Variant        Variant
	Segments       []string
	Duration       float64
}

func hash(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func resolve(base *url.URL, value string) (string, error) {
	ref, err := url.Parse(value)
	if err != nil || value == "" || len(value) > 16384 || strings.ContainsAny(value, "\r\n\x00") {
		return "", ErrUnsupported
	}
	u := base.ResolveReference(ref)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (base.Scheme == "https" && u.Scheme != "https") || len(u.String()) > 16384 {
		return "", ErrUnsupported
	}
	return u.String(), nil
}

// attributes handles quoted commas (not backslash escapes, which HLS does not
// define), rejects duplicate fields and never includes input in diagnostics.
func attributes(value string) (map[string]string, error) {
	result := map[string]string{}
	for value != "" {
		key, rest, ok := strings.Cut(value, "=")
		if !ok || key == "" {
			return nil, ErrUnsupported
		}
		for _, c := range key {
			if (c < 'A' || c > 'Z') && c != '-' && (c < '0' || c > '9') {
				return nil, ErrUnsupported
			}
		}
		var item string
		if strings.HasPrefix(rest, "\"") {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return nil, ErrUnsupported
			}
			item = rest[1 : 1+end]
			rest = rest[2+end:]
		} else {
			var comma bool
			item, rest, comma = strings.Cut(rest, ",")
			if comma && rest == "" {
				return nil, ErrUnsupported
			}
			if strings.ContainsAny(item, "\" \t") {
				return nil, ErrUnsupported
			}
			if rest != "" {
				rest = "," + rest
			}
		}
		if _, exists := result[key]; exists || item == "" {
			return nil, ErrUnsupported
		}
		result[key] = item
		if rest == "" {
			break
		}
		if rest[0] != ',' || len(rest) == 1 {
			return nil, ErrUnsupported
		}
		value = rest[1:]
	}
	return result, nil
}

// Parse accepts master playlists and immutable, ended MPEG-TS VOD media
// playlists. Rejecting known unsupported semantics is intentional; this is not
// a claim of RFC-wide playback support or a universal permissive HLS parser.
func Parse(body []byte, source string) (Playlist, error) {
	p := Playlist{}
	if len(body) > MaxPlaylist {
		return p, ErrLimit
	}
	p.Digest = hash(body)
	if !utf8.Valid(body) || strings.HasPrefix(string(body), "\ufeff") {
		return p, ErrUnsupported
	}
	base, err := url.Parse(source)
	if err != nil {
		return p, ErrUnsupported
	}
	if _, err = resolve(base, source); err != nil {
		return p, err
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	if strings.ContainsRune(text, '\r') {
		return p, ErrUnsupported
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "#EXTM3U" {
		return p, ErrUnsupported
	}
	var pending map[string]string
	var duration float64
	var target int64
	version := 1
	var maximumDuration float64
	var fractional bool
	var ended, vod, hasDuration bool
	seen := map[string]bool{}
	uriBytes := 0
	for _, line := range lines[1:] {
		if len(line) > 32768 {
			return p, ErrLimit
		}
		if line == "" {
			continue
		}
		if strings.TrimSpace(line) != line {
			return p, ErrUnsupported
		}
		if !strings.HasPrefix(line, "#") {
			if ended {
				return p, ErrUnsupported
			}
			uri, e := resolve(base, line)
			if e != nil {
				return p, e
			}
			uriBytes += len(uri)
			if uriBytes > 2<<20 {
				return p, ErrLimit
			}
			if pending != nil {
				if hasDuration || len(p.Segments) > 0 || target != 0 || vod {
					return p, ErrUnsupported
				}
				bandwidth, e := strconv.ParseInt(pending["BANDWIDTH"], 10, 64)
				if e != nil || bandwidth <= 0 || bandwidth > 1<<40 {
					return p, ErrUnsupported
				}
				if pending["AUDIO"] != "" || pending["VIDEO"] != "" || pending["SUBTITLES"] != "" || (pending["CLOSED-CAPTIONS"] != "" && pending["CLOSED-CAPTIONS"] != "NONE") {
					return p, ErrUnsupported
				}
				v := Variant{ID: hash([]byte(uri)), Bandwidth: bandwidth, Resolution: pending["RESOLUTION"], Codecs: pending["CODECS"], URL: uri}
				if err := validateVariant(v); err != nil {
					return p, err
				}
				p.Variants = append(p.Variants, v)
				pending = nil
			} else {
				if !hasDuration || len(p.Variants) > 0 {
					return p, ErrUnsupported
				}
				p.Segments = append(p.Segments, uri)
				p.Duration += duration
				hasDuration = false
			}
			if len(p.Variants) > 64 || len(p.Segments) > MaxObjects {
				return p, ErrLimit
			}
			continue
		}
		if !strings.HasPrefix(line, "#EXT") {
			continue
		}
		tag, value, _ := strings.Cut(line, ":")
		switch tag {
		case "#EXT-X-VERSION":
			var e error
			version, e = strconv.Atoi(value)
			if seen[tag] || e != nil || version < 1 || version > 3 {
				return p, ErrUnsupported
			}
			seen[tag] = true
		case "#EXT-X-STREAM-INF":
			if pending != nil || hasDuration || len(p.Segments) > 0 {
				return p, ErrUnsupported
			}
			pending, err = attributes(value)
			if err != nil {
				return p, err
			}
		case "#EXTINF":
			if hasDuration || pending != nil || ended || len(p.Variants) > 0 {
				return p, ErrUnsupported
			}
			number, _, ok := strings.Cut(value, ",")
			duration, err = strconv.ParseFloat(number, 64)
			if !ok || err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 86400 {
				return p, ErrUnsupported
			}
			hasDuration = true
			maximumDuration = max(maximumDuration, math.Round(duration))
			fractional = fractional || duration != math.Trunc(duration)
		case "#EXT-X-TARGETDURATION":
			if seen[tag] || len(p.Variants) > 0 || pending != nil {
				return p, ErrUnsupported
			}
			seen[tag] = true
			target, err = strconv.ParseInt(value, 10, 64)
			if err != nil || target <= 0 || target > 86400 {
				return p, ErrUnsupported
			}
		case "#EXT-X-MEDIA-SEQUENCE":
			n, e := strconv.ParseUint(value, 10, 64)
			if seen[tag] || len(p.Segments) > 0 || pending != nil || len(p.Variants) > 0 || e != nil || n > 1<<53 {
				return p, ErrUnsupported
			}
			seen[tag] = true
		case "#EXT-X-PLAYLIST-TYPE":
			if seen[tag] || value != "VOD" || pending != nil || len(p.Variants) > 0 {
				return p, ErrUnsupported
			}
			seen[tag] = true
			vod = true
		case "#EXT-X-ENDLIST":
			if seen[tag] || value != "" || pending != nil || hasDuration || len(p.Variants) > 0 {
				return p, ErrUnsupported
			}
			seen[tag] = true
			ended = true
		case "#EXT-X-KEY":
			fields, e := attributes(value)
			if e != nil || len(fields) != 1 || fields["METHOD"] != "NONE" || pending != nil || len(p.Variants) > 0 {
				return p, ErrUnsupported
			}
		default:
			return p, ErrUnsupported
		}
		if hasDuration && target > 0 && math.Round(duration) > float64(target) {
			return p, ErrUnsupported
		}
	}
	if pending != nil || hasDuration {
		return p, ErrUnsupported
	}
	if len(p.Variants) > 0 {
		return p, nil
	}
	if len(p.Segments) == 0 || !vod || !ended || target == 0 {
		return p, ErrUnsupported
	}
	// TARGETDURATION may legally precede or follow segment tags; validate all
	// declared durations independently of tag order.
	if maximumDuration > float64(target) || (fractional && version < 3) {
		return p, ErrUnsupported
	}
	return p, nil
}

func validateVariant(v Variant) error {
	if v.Resolution != "" {
		w, h, ok := strings.Cut(v.Resolution, "x")
		width, e1 := strconv.ParseUint(w, 10, 16)
		height, e2 := strconv.ParseUint(h, 10, 16)
		if !ok || e1 != nil || e2 != nil || width == 0 || height == 0 || width > 16384 || height > 16384 {
			return ErrUnsupported
		}
	}
	if v.Codecs != "" {
		seen := map[string]bool{}
		for _, codec := range strings.Split(v.Codecs, ",") {
			kind := "audio"
			if strings.HasPrefix(codec, "avc1.") {
				kind = "video"
				if len(codec) != 11 {
					return ErrUnsupported
				}
				if _, e := hex.DecodeString(codec[5:]); e != nil {
					return ErrUnsupported
				}
			} else if codec != "mp4a.40.2" {
				return ErrUnsupported
			}
			if seen[kind] {
				return ErrUnsupported
			}
			seen[kind] = true
		}
	}
	return nil
}
