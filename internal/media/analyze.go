package media

import (
	"bytes"
	"context"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"net/http"
	"time"
)

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxPlaylist {
		return 0, ErrLimit
	}
	return b.Buffer.Write(p)
}
func fetch(ctx context.Context, client *http.Client, source string) (Playlist, string, error) {
	if len(source) > 16384 {
		return Playlist{}, "", ErrLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var buffer boundedBuffer
	representation, err := engine.Stream(ctx, client, engine.Request{Source: source}, &buffer, nil, nil)
	if err != nil {
		return Playlist{}, "", err
	}
	p, err := Parse(buffer.Bytes(), representation.Effective)
	return p, representation.Effective, err
}

// Choice deliberately excludes playlist URLs and signed queries.
type Choice struct {
	ID         string `json:"id"`
	Bandwidth  int64  `json:"bandwidth"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
}

// Analyze returns selectable metadata while keeping request URLs private.
func Analyze(ctx context.Context, client *http.Client, source string) ([]Choice, error) {
	p, _, err := fetch(ctx, client, source)
	if err != nil {
		return nil, err
	}
	if len(p.Variants) == 0 {
		return []Choice{{ID: "", Bandwidth: 0, Resolution: "Source playlist"}}, nil
	}
	choices := make([]Choice, 0, len(p.Variants))
	for _, v := range p.Variants {
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
