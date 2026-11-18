package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

const simpleMPD = `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT3S"><Period><AdaptationSet mimeType="video/mp4" codecs="avc1.42c01e" width="128" height="72"><SegmentTemplate timescale="10" duration="10" initialization="init-$RepresentationID$.mp4" media="$RepresentationID$-$Number%03d$.m4s"/><Representation id="v" bandwidth="1000"/></AdaptationSet></Period></MPD>`

func TestDASHAddressingSelectionAndRealManifest(t *testing.T) {
	plans, err := ParseDASH([]byte(simpleMPD), "https://example.invalid/path/index.mpd?secret=yes")
	if err != nil || len(plans) != 1 {
		t.Fatal(plans, err)
	}
	p := plans[0]
	if len(p.Segments) != 4 || p.Segments[1] != "https://example.invalid/path/v-001.m4s" || p.Tracks[0] != (Track{Kind: "video", End: 4}) {
		t.Fatal(p)
	}
	for _, timeline := range []string{`<S t="0" d="10" r="2"/>`, `<S d="10" r="-1"/>`, `<S d="10" r="-1"/><S t="20" d="10"/>`} {
		body := strings.Replace(simpleMPD, ` duration="10"`, "", 1)
		body = strings.Replace(body, `media="$RepresentationID$-$Number%03d$.m4s"/>`, `media="$RepresentationID$-$Time$.m4s"><SegmentTimeline>`+timeline+`</SegmentTimeline></SegmentTemplate>`, 1)
		plans, err = ParseDASH([]byte(body), "https://example.invalid/index.mpd")
		if err != nil || !strings.HasSuffix(plans[0].Segments[3], "v-20.m4s") {
			t.Fatal(plans, err)
		}
	}
	body, err := os.ReadFile("../../testdata/dash/manifest.mpd")
	if err != nil {
		t.Fatal(err)
	}
	plans, err = ParseDASH(body, "https://example.invalid/index.mpd")
	if err != nil || len(plans) != 4 {
		t.Fatal(plans, err)
	}
	if plans[0].Variant.Resolution != "128x72 audio eng" || plans[1].Variant.Resolution != "128x72 audio fra" || len(plans[0].Tracks) != 2 || len(plans[0].Segments) != 6 {
		t.Fatal(plans)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	choices, err := Analyze(context.Background(), server.Client(), server.URL)
	if err != nil || len(choices) != 4 || strings.Contains(choices[0].Resolution, "http") {
		t.Fatal(choices, err)
	}
	selected, err := ResolveKind(context.Background(), server.Client(), server.URL, choices[1].ID, "dash")
	if err != nil || selected.Variant.ID != choices[1].ID {
		t.Fatal(selected, err)
	}
	selected, err = ResolveKind(context.Background(), server.Client(), server.URL, "", "dash")
	if err != nil || !strings.Contains(selected.Variant.Resolution, "256x144") {
		t.Fatal(selected, err)
	}
	if _, err = ResolveKind(context.Background(), server.Client(), server.URL, strings.Repeat("f", 64), "dash"); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestDASHRejectsUnsupportedSemanticsAndBounds(t *testing.T) {
	replacements := [][2]string{
		{`type="static"`, `type="dynamic"`}, {`PT3S`, `P1D`}, {`PT3S`, `PT0S`}, {`PT3S`, `PT90000S`},
		{`<Period>`, `<Period start="PT1S">`}, {`<Period>`, `<Period duration="PT2S">`}, {`</Period>`, `</Period><Period/>`},
		{`<Period>`, `<Period><ContentProtection/>`}, {`<Period>`, `<Period><EventStream/>`}, {`<Period>`, `<Period xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="https://example.invalid" >`},
		{`<Period>`, `<Period><BaseURL>http://downgrade.invalid/</BaseURL>`}, {`<Period>`, `<Period><BaseURL>file:///private</BaseURL>`}, {`<Period>`, `<Period><BaseURL>x/</BaseURL><BaseURL>y/</BaseURL>`},
		{`<SegmentTemplate`, `<SegmentList`}, {`<SegmentTemplate`, `<SegmentTemplate presentationTimeOffset="1"`},
		{`timescale="10"`, `timescale="0"`}, {`timescale="10"`, `timescale="-1"`}, {`timescale="10"`, `timescale="1000000001"`},
		{`duration="10"`, `duration="0"`}, {`duration="10"`, `duration="1" timescale="bad"`}, {`duration="10"`, `duration="9999999999999999999"`},
		{`id="v"`, `id="../v"`}, {`id="v"`, `id=""`}, {`bandwidth="1000"`, `bandwidth="0"`}, {`width="128"`, `width="0"`}, {`avc1.42c01e`, `hvc1.1`},
		{`<Representation`, `<Representation unknown="ignored"`}, {`<Representation`, `<Representation xmlns="evil"`},
		{`<Period>`, `<Period>text`}, {`</MPD>`, `</MPD>extra`}, {`<MPD`, `<!DOCTYPE MPD><MPD`}, {`</MPD>`, `</MPD><MPD/>`},
		{`<Representation id="v" bandwidth="1000"/>`, ``}, {`<AdaptationSet`, `<AdaptationSet lang="secret query"`},
		{`initialization="init-$RepresentationID$.mp4"`, `initialization="init-$Number$.mp4"`}, {`$Number%03d$`, `$Number%099d$`},
		{`$Number%03d$`, `$Unknown$`}, {`$Number%03d$`, `$Number`}, {`$Number%03d$`, `constant`}, {`$Number%03d$`, `$Bandwidth$`},
	}
	for _, pair := range replacements {
		t.Run(pair[1], func(t *testing.T) {
			body := strings.Replace(simpleMPD, pair[0], pair[1], 1)
			if _, err := ParseDASH([]byte(body), "https://example.invalid/list.mpd"); err == nil {
				t.Fatal("accepted unsupported semantics", body)
			}
		})
	}
	if _, err := ParseDASH(make([]byte, MaxPlaylist+1), "https://example.invalid"); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for _, timeline := range []string{`<S d="0"/>`, `<S t="1" d="10"/>`, `<S d="10" r="-2"/>`, `<S d="10" r="2048"/>`, `<S d="10"/>`, `<S d="10" r="-1"/><S d="10"/>`, `<S d="10"/><S t="0" d="10"/>`, `<S d="10" r="-1"/><S t="15" d="10"/>`, `<S d="10" r="3"/>`} {
		body := strings.Replace(simpleMPD, ` duration="10"`, "", 1)
		body = strings.Replace(body, `media="$RepresentationID$-$Number%03d$.m4s"/>`, `media="$RepresentationID$-$Number%03d$.m4s"><SegmentTimeline>`+timeline+`</SegmentTimeline></SegmentTemplate>`, 1)
		if _, err := ParseDASH([]byte(body), "https://example.invalid"); err == nil {
			t.Fatal("accepted invalid timeline", timeline)
		}
	}
	body := strings.Replace(simpleMPD, `duration="10"`, `duration="1"`, 1)
	body = strings.Replace(body, `PT3S`, `PT86400S`, 1)
	if _, err := ParseDASH([]byte(body), "https://example.invalid"); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestDASHInheritanceAudioAndTemplateEscapes(t *testing.T) {
	body := strings.Replace(simpleMPD, `<Period>`, `<BaseURL>cdn/</BaseURL><Period><BaseURL>assets/</BaseURL>`, 1)
	body = strings.Replace(body, `<AdaptationSet`, `<AdaptationSet`, 1)
	body = strings.Replace(body, `<Representation id="v" bandwidth="1000"/>`, `<Representation id="v" bandwidth="1000"><BaseURL>v/</BaseURL><SegmentTemplate startNumber="7"/></Representation>`, 1)
	p, err := ParseDASH([]byte(body), "https://example.invalid/root/index.mpd")
	if err != nil || p[0].Segments[1] != "https://example.invalid/root/cdn/assets/v/v-007.m4s" {
		t.Fatal(p, err)
	}
	body = strings.ReplaceAll(simpleMPD, "video/mp4", "audio/mp4")
	body = strings.ReplaceAll(body, "avc1.42c01e", "mp4a.40.2")
	p, err = ParseDASH([]byte(body), "https://example.invalid")
	if err != nil || p[0].Tracks[0].Kind != "audio" {
		t.Fatal(p, err)
	}
	body = strings.Replace(simpleMPD, ` mediaPresentationDuration="PT3S"`, "", 1)
	body = strings.Replace(body, `<Period>`, `<Period duration="PT3S">`, 1)
	if _, err = ParseDASH([]byte(body), "https://example.invalid"); err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://example.invalid/root/")
	uri, err := dashURL(base, `$$-$RepresentationID$-$Bandwidth%04d$-$Time$.m4s`, "v", 12, 1, 20, false)
	if err != nil || uri != "https://example.invalid/root/$-v-0012-20.m4s" {
		t.Fatal(uri, err)
	}
}

func FuzzDASHManifest(f *testing.F) {
	f.Add([]byte(simpleMPD))
	f.Add([]byte(`<MPD/>`))
	f.Fuzz(func(t *testing.T, body []byte) { _, _ = ParseDASH(body, "https://example.invalid/manifest.mpd") })
}

func TestDASHFractionalDurationsUseExactPeriodTicks(t *testing.T) {
	for _, test := range []struct {
		duration, scale, segment string
		objects                  int
	}{
		{"PT1.1S", "100", "110", 2},
		{"PT0H0M1.10S", "100", "110", 2},
		{"PT0.1S", "10", "1", 2},
		{"PT0.000000001S", "1000000000", "1", 2},
		// A real fractional tick still rounds upward and requires a second fragment.
		{"PT1.1001S", "100", "110", 3},
	} {
		t.Run(test.duration, func(t *testing.T) {
			base := strings.Replace(simpleMPD, `PT3S`, test.duration, 1)
			base = strings.Replace(base, `timescale="10"`, `timescale="`+test.scale+`"`, 1)
			for _, timeline := range []bool{false, true} {
				body := strings.Replace(base, `duration="10"`, `duration="`+test.segment+`"`, 1)
				if timeline {
					body = strings.Replace(body, ` duration="`+test.segment+`"`, "", 1)
					body = strings.Replace(body, `media="$RepresentationID$-$Number%03d$.m4s"/>`, `media="$RepresentationID$-$Number%03d$.m4s"><SegmentTimeline><S d="`+test.segment+`" r="-1"/></SegmentTimeline></SegmentTemplate>`, 1)
				}
				plans, err := ParseDASH([]byte(body), "https://example.invalid/index.mpd")
				if err != nil || len(plans) != 1 || len(plans[0].Segments) != test.objects {
					t.Fatal(timeline, plans, err)
				}
			}
			// A finite one-segment timeline is complete only when it covers the exact end.
			body := strings.Replace(base, ` duration="10"`, "", 1)
			body = strings.Replace(body, `media="$RepresentationID$-$Number%03d$.m4s"/>`, `media="$RepresentationID$-$Number%03d$.m4s"><SegmentTimeline><S d="`+test.segment+`"/></SegmentTimeline></SegmentTemplate>`, 1)
			_, err := ParseDASH([]byte(body), "https://example.invalid/index.mpd")
			if (err == nil) != (test.objects == 2) {
				t.Fatal("wrong exact finite timeline boundary", err)
			}
		})
	}
}
