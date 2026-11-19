package media

import (
	"bytes"
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"net/http"
	"strings"
	"time"
)

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxPlaylist {
		return 0, ErrLimit
	}
	return b.Buffer.Write(p)
}
func fetchDocument(ctx context.Context, client *http.Client, source string) ([]byte, string, error) {
	if len(source) > 16384 {
		return nil, "", ErrLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var buffer boundedBuffer
	representation, err := engine.Stream(ctx, client, engine.Request{Source: source}, &buffer, nil, nil)
	if err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), representation.Effective, nil
}

func fetch(ctx context.Context, client *http.Client, source string) (Playlist, string, error) {
	body, effective, err := fetchDocument(ctx, client, source)
	if err != nil {
		return Playlist{}, "", err
	}
	p, err := Parse(body, effective)
	return p, effective, err
}
func dashDocument(body []byte) bool { return strings.HasPrefix(strings.TrimSpace(string(body)), "<") }

// Choice deliberately excludes playlist URLs and signed queries.
type Choice struct {
	ID         string `json:"id"`
	Bandwidth  int64  `json:"bandwidth"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
}

// Analyze returns selectable metadata while keeping request URLs private.
func Analyze(ctx context.Context, client *http.Client, source string) ([]Choice, error) {
	body, effective, err := fetchDocument(ctx, client, source)
	if err != nil {
		return nil, err
	}
	var variants []Variant
	if dashDocument(body) {
		plans, e := ParseDASH(body, effective)
		if e != nil {
			return nil, e
		}
		for _, plan := range plans {
			variants = append(variants, plan.Variant)
		}
	} else {
		p, e := Parse(body, effective)
		if e != nil {
			return nil, e
		}
		variants = p.Variants
	}
	if len(variants) == 0 {
		return []Choice{{ID: "", Bandwidth: 0, Resolution: "Source playlist"}}, nil
	}
	choices := make([]Choice, 0, len(variants))
	for _, v := range variants {
		choices = append(choices, Choice{v.ID, v.Bandwidth, v.Resolution, v.Codecs})
	}
	return choices, nil
}

// Resolve chooses an exact variant URI fingerprint or the highest declared
// bandwidth, then requires an ended, immutable media playlist (no nested masters).
func Resolve(ctx context.Context, client *http.Client, source, choice string) (Plan, error) {
	p, effective, err := fetch(ctx, client, source)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{SourceDigest: p.Digest, PlaylistDigest: p.Digest, PlaylistURL: effective, Segments: p.Segments, Duration: p.Duration}
	if len(p.Variants) == 0 {
		if choice != "" {
			return Plan{}, ErrUnsupported
		}
		return plan, nil
	}
	var selected Variant
	for _, v := range p.Variants {
		if (choice != "" && v.ID == choice) || (choice == "" && v.Bandwidth > selected.Bandwidth) {
			selected = v
		}
	}
	if selected.URL == "" {
		return Plan{}, ErrUnsupported
	}
	child, uri, err := fetch(ctx, client, selected.URL)
	if err != nil {
		return Plan{}, err
	}
	if len(child.Variants) > 0 {
		return Plan{}, ErrUnsupported
	}
	plan.PlaylistDigest = child.Digest
	plan.PlaylistURL = uri
	plan.Variant = selected
	plan.Segments = child.Segments
	plan.Duration = child.Duration
	return plan, nil
}

// ResolveKind keeps explicit protocol admission from changing semantics when a
// source changes format; analysis alone may inspect either manifest type.
func ResolveKind(ctx context.Context, client *http.Client, source, choice, kind string) (Plan, error) {
	if kind == "hls" {
		return Resolve(ctx, client, source, choice)
	}
	body, effective, err := fetchDocument(ctx, client, source)
	if err != nil {
		return Plan{}, err
	}
	plans, err := ParseDASH(body, effective)
	if err != nil {
		return Plan{}, err
	}
	var selected Plan
	for _, p := range plans {
		if (choice != "" && p.Variant.ID == choice) || (choice == "" && p.Variant.Bandwidth > selected.Variant.Bandwidth) {
			selected = p
		}
	}
	if len(selected.Tracks) == 0 {
		return Plan{}, ErrUnsupported
	}
	return selected, nil
}
