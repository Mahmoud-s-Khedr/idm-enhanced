package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/engine"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const vod = "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MEDIA-SEQUENCE:7\n#EXTINF:1.5,First\na.ts\n#EXTINF:2,Second\n../b.ts?token=private\n#EXT-X-ENDLIST\n"
const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000,RESOLUTION=128x72,CODECS=\"avc1.42e01e,mp4a.40.2\"\nlow/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2000,RESOLUTION=256x144,CODECS=\"avc1.42e01e,mp4a.40.2\",CLOSED-CAPTIONS=NONE\nhigh/index.m3u8?token=private\n"

func TestPlaylistParsesRelativeVODAndQuotedVariantAttributes(t *testing.T) {
	p, err := Parse([]byte(vod), "https://example.invalid/dir/index.m3u8")
	if err != nil || p.Duration != 3.5 || len(p.Segments) != 2 || p.Segments[0] != "https://example.invalid/dir/a.ts" || p.Segments[1] != "https://example.invalid/b.ts?token=private" {
		t.Fatal(p, err)
	}
	p, err = Parse([]byte(master), "https://example.invalid/master.m3u8")
	if err != nil || len(p.Variants) != 2 || p.Variants[0].Bandwidth != 1000 || p.Variants[1].URL != "https://example.invalid/high/index.m3u8?token=private" || len(p.Variants[1].ID) != 64 {
		t.Fatal(p, err)
	}
	for _, text := range []string{strings.ReplaceAll(vod, "\n", "\r\n"), strings.Replace(vod, "#EXTINF:1.5", "# comment\n#EXT-X-KEY:METHOD=NONE\n#EXTINF:1.5", 1), strings.Replace(vod, "#EXT-X-MEDIA-SEQUENCE:7\n", "", 1)} {
		if _, err := Parse([]byte(text), "https://example.invalid/list"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlaylistRejectsUnsupportedAndMalformedSources(t *testing.T) {
	cases := map[string]string{
		"not playlist": "text", "bom": "\ufeff" + vod, "invalid utf8": vod + string([]byte{255}), "bare CR": strings.Replace(vod, "a.ts", "a.t\rs", 1),
		"whitespace": strings.Replace(vod, "a.ts", " a.ts", 1), "dangling duration": strings.Replace(vod, "a.ts\n", "", 1),
		"orphan uri": strings.Replace(vod, "#EXTINF:1.5,First\n", "", 1), "after end": vod + "x.ts\n",
		"live": strings.Replace(vod, "#EXT-X-ENDLIST\n", "", 1), "event": strings.Replace(vod, "TYPE:VOD", "TYPE:EVENT", 1),
		"untyped": strings.Replace(vod, "#EXT-X-PLAYLIST-TYPE:VOD\n", "", 1), "missing target": strings.Replace(vod, "#EXT-X-TARGETDURATION:2\n", "", 1),
		"zero target": strings.Replace(vod, "TARGETDURATION:2", "TARGETDURATION:0", 1), "invalid target": strings.Replace(vod, "TARGETDURATION:2", "TARGETDURATION:bad", 1),
		"short target": strings.Replace(vod, "TARGETDURATION:2", "TARGETDURATION:1", 1), "late short target": strings.Replace(strings.Replace(vod, "#EXT-X-TARGETDURATION:2\n", "", 1), "#EXT-X-ENDLIST", "#EXT-X-TARGETDURATION:1\n#EXT-X-ENDLIST", 1),
		"version": strings.Replace(vod, "VERSION:3", "VERSION:6", 1), "bad version": strings.Replace(vod, "VERSION:3", "VERSION:bad", 1), "fractional v1": strings.Replace(vod, "#EXT-X-VERSION:3\n", "", 1),
		"duplicate target":   strings.Replace(vod, "#EXT-X-TARGETDURATION:2", "#EXT-X-TARGETDURATION:2\n#EXT-X-TARGETDURATION:2", 1),
		"duplicate version":  strings.Replace(vod, "#EXT-X-VERSION:3", "#EXT-X-VERSION:3\n#EXT-X-VERSION:3", 1),
		"duplicate type":     strings.Replace(vod, "#EXT-X-PLAYLIST-TYPE:VOD", "#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-PLAYLIST-TYPE:VOD", 1),
		"duplicate sequence": strings.Replace(vod, "#EXT-X-MEDIA-SEQUENCE:7", "#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-MEDIA-SEQUENCE:7", 1),
		"negative sequence":  strings.Replace(vod, "SEQUENCE:7", "SEQUENCE:-1", 1), "late sequence": strings.Replace(strings.Replace(vod, "#EXT-X-MEDIA-SEQUENCE:7\n", "", 1), "#EXT-X-ENDLIST", "#EXT-X-MEDIA-SEQUENCE:7\n#EXT-X-ENDLIST", 1),
		"duplicate end": vod + "#EXT-X-ENDLIST\n", "end with data": strings.Replace(vod, "#EXT-X-ENDLIST", "#EXT-X-ENDLIST:bad", 1),
		"NaN": strings.Replace(vod, "1.5,First", "NaN,First", 1), "infinite": strings.Replace(vod, "1.5,First", "Inf,First", 1), "negative": strings.Replace(vod, "1.5,First", "-1,First", 1),
		"invalid duration": strings.Replace(vod, "1.5,First", "bad,First", 1), "missing comma": strings.Replace(vod, "1.5,First", "1.5", 1),
		"two durations": strings.Replace(vod, "a.ts", "#EXTINF:1,Other\na.ts", 1), "dangling": "#EXTM3U\n#EXTINF:1,\n",
		"encryption": strings.Replace(vod, "a.ts", "#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\na.ts", 1),
		"map":        strings.Replace(vod, "a.ts", "#EXT-X-MAP:URI=\"init.mp4\"\na.ts", 1), "range": strings.Replace(vod, "a.ts", "#EXT-X-BYTERANGE:10@0\na.ts", 1),
		"discontinuity": strings.Replace(vod, "a.ts", "#EXT-X-DISCONTINUITY\na.ts", 1), "unknown semantics": vod + "#EXT-X-GAP\n",
		"uri scheme": strings.Replace(vod, "a.ts", "file:///private", 1), "uri user": strings.Replace(vod, "a.ts", "https://name:secret@example.invalid/a", 1),
		"uri fragment": strings.Replace(vod, "a.ts", "a.ts#secret", 1), "downgrade": strings.Replace(vod, "a.ts", "http://example.invalid/a", 1), "bad URI": strings.Replace(vod, "a.ts", "http://[", 1),
		"master media mix": master + "#EXT-X-TARGETDURATION:2\n", "missing bandwidth": strings.Replace(master, "BANDWIDTH=1000,", "", 1),
		"zero bandwidth": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=0", 1), "too much bandwidth": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=99999999999999", 1),
		"alternate audio":   strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=1000,AUDIO=\"other\"", 1),
		"external captions": strings.Replace(master, "CLOSED-CAPTIONS=NONE", "CLOSED-CAPTIONS=\"group\"", 1),
		"bad resolution":    strings.Replace(master, "128x72", "bad", 1), "zero resolution": strings.Replace(master, "128x72", "0x72", 1),
		"unsupported codec": strings.Replace(master, "avc1.42e01e", "hvc1.123", 1), "bad avc": strings.Replace(master, "avc1.42e01e", "avc1.zzzzzz", 1), "short avc": strings.Replace(master, "avc1.42e01e", "avc1.42", 1),
		"duplicate codecs": strings.Replace(master, "avc1.42e01e,mp4a.40.2", "mp4a.40.2,mp4a.40.2", 1),
		"duplicate attr":   strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=1000,BANDWIDTH=1", 1), "empty attr": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=", 1),
		"lower attr": strings.Replace(master, "BANDWIDTH=1000", "bandwidth=1000", 1), "missing equals": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH", 1),
		"unterminated quotes": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=\"1000", 1), "quoted junk": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH=\"1000\"junk", 1),
		"space attr": strings.Replace(master, "BANDWIDTH=1000", "BANDWIDTH= 1000", 1), "trailing comma": "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,\na\n",
		"two stream tags": "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n#EXT-X-STREAM-INF:BANDWIDTH=2\na\n", "dangling stream": "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n",
		"empty": "#EXTM3U\n", "master key": master + "#EXT-X-KEY:METHOD=NONE\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body), "https://example.invalid/list"); err == nil {
				t.Fatal("accepted unsupported source")
			}
		})
	}
	for _, source := range []string{"", ":bad", "file:///list", "https://name:secret@example.invalid/list"} {
		if _, err := Parse([]byte(vod), source); err == nil {
			t.Fatal("unsafe source", source)
		}
	}
}

func TestPlaylistBoundsAndRedactedErrors(t *testing.T) {
	cases := []string{strings.Repeat("x", MaxPlaylist+1), "#EXTM3U\n#" + strings.Repeat("x", 32769), "#EXTM3U\n" + strings.Repeat("#EXT-X-STREAM-INF:BANDWIDTH=1\nx\n", 65), "#EXTM3U\n" + strings.Repeat("#EXTINF:1,\nx\n", MaxObjects+1)}
	// A long base can amplify short relative references; bound resolved metadata.
	cases = append(cases, "#EXTM3U\n"+strings.Repeat("#EXTINF:1,\nx\n", 300))
	for i, body := range cases {
		base := "https://example.invalid/list"
		if i == 4 {
			base = "https://example.invalid/" + strings.Repeat("a", 10000) + "/list"
		}
		_, err := Parse([]byte(body), base)
		if !errors.Is(err, ErrLimit) {
			t.Fatalf("%d: %v", i, err)
		}
	}
	_, err := Parse([]byte(strings.Replace(vod, "a.ts", "https://name:private@example.invalid/a", 1)), "https://example.invalid/list?token=private")
	if err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal(err)
	}
}

func TestAnalyzeResolveRedirectsSelectionAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/root/master", http.StatusFound)
		case "/root/master":
			_, _ = fmt.Fprint(w, master)
		case "/root/low/index.m3u8", "/root/high/index.m3u8":
			_, _ = fmt.Fprint(w, vod)
		case "/nested":
			_, _ = fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n/root/master\n")
		case "/large":
			_, _ = fmt.Fprint(w, strings.Repeat("x", MaxPlaylist+1))
		default:
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	choices, err := Analyze(ctx, server.Client(), server.URL+"/start")
	if err != nil || len(choices) != 2 {
		t.Fatal(choices, err)
	}
	encoded, _ := json.Marshal(choices)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "http") {
		t.Fatal("choice leaked request metadata")
	}
	p, err := Resolve(ctx, server.Client(), server.URL+"/start", "")
	if err != nil || p.Variant.Bandwidth != 2000 || !strings.HasSuffix(p.Segments[0], "/root/high/a.ts") {
		t.Fatal(p, err)
	}
	p, err = Resolve(ctx, server.Client(), server.URL+"/start", choices[0].ID)
	if err != nil || p.Variant.Bandwidth != 1000 {
		t.Fatal(p, err)
	}
	single := server.URL + "/root/low/index.m3u8"
	choices, err = Analyze(ctx, server.Client(), single)
	if err != nil || len(choices) != 1 || choices[0].ID != "" {
		t.Fatal(choices, err)
	}
	if _, err = Resolve(ctx, server.Client(), single, ""); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ path, choice string }{{"/start", "not-a-choice"}, {"/root/low/index.m3u8", "not-a-choice"}, {"/nested", ""}, {"/missing", ""}, {"/large", ""}} {
		if _, err = Resolve(ctx, server.Client(), server.URL+input.path, input.choice); err == nil {
			t.Fatal(input)
		}
	}
	if _, err = Analyze(ctx, server.Client(), server.URL+"/missing"); err == nil {
		t.Fatal("ignored HTTP failure")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = Analyze(cancelled, server.Client(), server.URL+"/start"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestObjectDownloadsShareHTTPChecksAndRejectOversizedHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			w.Header().Set("Content-Length", fmt.Sprint(MaxObjectBytes+1))
			return
		}
		_, _ = io.WriteString(w, "object")
	}))
	defer server.Close()
	var body bytes.Buffer
	rep, err := DownloadObject(context.Background(), server.Client(), server.URL+"/ok", &body, nil)
	if err != nil || body.String() != "object" || rep.Length != 6 {
		t.Fatal(body.String(), rep, err)
	}
	if _, err = DownloadObject(context.Background(), server.Client(), server.URL+"/large", io.Discard, nil); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	denied := errors.New("identity changed")
	if _, err = DownloadObject(context.Background(), server.Client(), server.URL+"/ok", io.Discard, func(engine.Representation) error { return denied }); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if _, err = Analyze(context.Background(), server.Client(), strings.Repeat("x", 16385)); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
