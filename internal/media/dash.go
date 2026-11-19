package media

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Track describes a contiguous initialization+fragment sequence in Plan.Segments.
// Ends are exclusive, so audio and video never share a demuxer input.
type Track struct {
	Kind string
	End  int
}

type dashNode struct {
	name     string
	attrs    map[string]string
	children []*dashNode
	text     strings.Builder
}

func (n *dashNode) childrenNamed(name string) []*dashNode {
	var result []*dashNode
	for _, c := range n.children {
		if c.name == name {
			result = append(result, c)
		}
	}
	return result
}
func (n *dashNode) one(name string) (*dashNode, error) {
	list := n.childrenNamed(name)
	if len(list) > 1 {
		return nil, ErrUnsupported
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// A closed grammar prevents silently ignoring encryption, XLink, segment ranges,
// multi-period timing or other semantics that the downloader cannot preserve.
var dashGrammar = map[string]string{
	"MPD": "Period BaseURL ProgramInformation ServiceDescription", "Period": "AdaptationSet BaseURL SegmentTemplate",
	"AdaptationSet": "Representation BaseURL SegmentTemplate", "Representation": "BaseURL SegmentTemplate AudioChannelConfiguration",
	"SegmentTemplate": "SegmentTimeline", "SegmentTimeline": "S", "S": "", "BaseURL": "",
	"ProgramInformation": "Title Source Copyright", "Title": "", "Source": "", "Copyright": "", "AudioChannelConfiguration": "", "ServiceDescription": "",
}
var dashAttributes = map[string]string{
	"ServiceDescription": "id",
	"MPD":                "type mediaPresentationDuration minBufferTime profiles maxSegmentDuration schemaLocation",
	"Period":             "id start duration", "AdaptationSet": "id contentType mimeType codecs lang width height frameRate audioSamplingRate segmentAlignment startWithSAP bitstreamSwitching par maxWidth maxHeight maxFrameRate",
	"Representation":  "id bandwidth mimeType codecs width height frameRate audioSamplingRate sar startWithSAP",
	"SegmentTemplate": "timescale duration startNumber initialization media presentationTimeOffset",
	"S":               "t d r", "BaseURL": "", "ProgramInformation": "lang moreInformationURL", "Title": "", "Source": "", "Copyright": "",
	"AudioChannelConfiguration": "schemeIdUri value",
}

func word(list, value string) bool { return strings.Contains(" "+list+" ", " "+value+" ") }
func parseDASHXML(body []byte) (*dashNode, error) {
	if len(body) > MaxPlaylist {
		return nil, ErrLimit
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var root *dashNode
	var stack []*dashNode
	count := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrUnsupported
		}
		switch t := token.(type) {
		case xml.StartElement:
			count++
			if count > 16384 || len(stack) >= 16 {
				return nil, ErrLimit
			}
			_, ok := dashGrammar[t.Name.Local]
			if !ok || t.Name.Space != "urn:mpeg:dash:schema:mpd:2011" {
				return nil, ErrUnsupported
			}
			n := &dashNode{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
					continue
				}
				allowedSpace := a.Name.Space == "" || a.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" && a.Name.Local == "schemaLocation" || a.Name.Space == "http://www.w3.org/XML/1998/namespace" && a.Name.Local == "lang"
				if !allowedSpace || !word(dashAttributes[n.name], a.Name.Local) {
					return nil, ErrUnsupported
				}
				if _, exists := n.attrs[a.Name.Local]; exists {
					return nil, ErrUnsupported
				}
				n.attrs[a.Name.Local] = a.Value
			}
			if len(stack) == 0 {
				if root != nil || n.name != "MPD" {
					return nil, ErrUnsupported
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				if !word(dashGrammar[parent.name], n.name) {
					return nil, ErrUnsupported
				}
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(t)) != "" {
				return nil, ErrUnsupported
			}
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				if n.name != "BaseURL" && n.name != "Title" && n.name != "Source" && n.name != "Copyright" && strings.TrimSpace(string(t)) != "" {
					return nil, ErrUnsupported
				}
				_, _ = n.text.Write(t)
			}
		case xml.Directive:
			return nil, ErrUnsupported
		case xml.ProcInst:
			if t.Target != "xml" || root != nil {
				return nil, ErrUnsupported
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, ErrUnsupported
	}
	return root, nil
}

var dashDuration = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$`)

// MPD decimal durations must remain exact until conversion to timeline ticks.
// Float64 is used only for the display/checkpoint duration summary.
func parseDuration(value string) (*big.Rat, error) {
	if len(value) > 128 {
		return nil, ErrUnsupported
	}
	parts := dashDuration.FindStringSubmatch(value)
	if parts == nil || value == "PT" {
		return nil, ErrUnsupported
	}
	total := new(big.Rat)
	for i, scale := range []int64{3600, 60, 1} {
		if parts[i+1] != "" {
			n, ok := new(big.Rat).SetString(parts[i+1])
			if !ok {
				return nil, ErrUnsupported
			}
			total.Add(total, n.Mul(n, new(big.Rat).SetInt64(scale)))
		}
	}
	if total.Sign() < 0 || total.Cmp(new(big.Rat).SetInt64(86400)) > 0 {
		return nil, ErrUnsupported
	}
	return total, nil
}

func dashEndTick(duration *big.Rat, scale int64) (int64, bool) {
	scaled := new(big.Rat).Mul(duration, new(big.Rat).SetInt64(scale))
	ticks, remainder := new(big.Int), new(big.Int)
	ticks.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	if remainder.Sign() > 0 {
		ticks.Add(ticks, big.NewInt(1))
	}
	// Duration and scale have already been bounded to 86,400 and 1e9.
	return ticks.Int64(), remainder.Sign() == 0
}

func dashInt(value string, fallback int64) (int64, error) {
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || n > 1<<53 {
		return 0, ErrUnsupported
	}
	return n, nil
}
func dashBase(parent string, n *dashNode) (string, error) {
	child, err := n.one("BaseURL")
	if err != nil {
		return "", err
	}
	if child == nil {
		return parent, nil
	}
	base, err := url.Parse(parent)
	if err != nil {
		return "", ErrUnsupported
	}
	return resolve(base, strings.TrimSpace(child.text.String()))
}
func dashTemplate(parent, n *dashNode) (*dashNode, error) {
	child, err := n.one("SegmentTemplate")
	if err != nil {
		return nil, err
	}
	if child == nil {
		return parent, nil
	}
	merged := &dashNode{name: "SegmentTemplate", attrs: map[string]string{}, children: child.children}
	if parent != nil {
		for k, v := range parent.attrs {
			merged.attrs[k] = v
		}
		if len(child.children) == 0 {
			merged.children = parent.children
		}
	}
	for k, v := range child.attrs {
		merged.attrs[k] = v
	}
	return merged, nil
}
func inherit(rep, adapt *dashNode, key string) string {
	if v := rep.attrs[key]; v != "" {
		return v
	}
	return adapt.attrs[key]
}

type dashRepresentation struct {
	id, kind, language, codecs, resolution string
	bandwidth                              int64
	objects                                []string
}

func dashRepresentationPlan(adapt, rep, template *dashNode, base string, duration *big.Rat) (dashRepresentation, error) {
	r := dashRepresentation{id: rep.attrs["id"], language: adapt.attrs["lang"], codecs: inherit(rep, adapt, "codecs")}
	if len(r.id) == 0 || len(r.id) > 128 || strings.ContainsAny(r.id, "/$%?#\\\x00\r\n") || len(r.language) > 35 {
		return r, ErrUnsupported
	}
	for _, c := range r.language {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-'
		if !valid {
			return r, ErrUnsupported
		}
	}
	mime := inherit(rep, adapt, "mimeType")
	r.kind = strings.TrimSuffix(mime, "/mp4")
	if (r.kind != "video" && r.kind != "audio") || (adapt.attrs["contentType"] != "" && adapt.attrs["contentType"] != r.kind) {
		return r, ErrUnsupported
	}
	var err error
	r.bandwidth, err = dashInt(rep.attrs["bandwidth"], 0)
	if err != nil || r.bandwidth <= 0 || r.bandwidth > 1<<40 {
		return r, ErrUnsupported
	}
	if r.kind == "video" {
		v := Variant{Codecs: r.codecs, Resolution: inherit(rep, adapt, "width") + "x" + inherit(rep, adapt, "height")}
		if !strings.HasPrefix(r.codecs, "avc1.") || strings.Contains(r.codecs, ",") || validateVariant(v) != nil {
			return r, ErrUnsupported
		}
		r.resolution = v.Resolution
	} else if r.codecs != "mp4a.40.2" {
		return r, ErrUnsupported
	}
	if channel, e := rep.one("AudioChannelConfiguration"); e != nil || (channel != nil && (r.kind != "audio" || channel.attrs["schemeIdUri"] != "urn:mpeg:dash:23003:3:audio_channel_configuration:2011" || (channel.attrs["value"] != "1" && channel.attrs["value"] != "2"))) {
		return r, ErrUnsupported
	}
	r.objects, err = dashObjects(template, base, r.id, r.bandwidth, duration)
	return r, err
}

// ParseDASH returns bounded selectable audio/video combinations. Metadata never
// contains request URLs; initialization is the first object of every track.
func ParseDASH(body []byte, source string) ([]Plan, error) {
	root, err := parseDASHXML(body)
	if err != nil {
		return nil, err
	}
	if root.attrs["type"] != "static" {
		return nil, ErrUnsupported
	}
	periods := root.childrenNamed("Period")
	if len(periods) != 1 {
		return nil, ErrUnsupported
	}
	p := periods[0]
	if p.attrs["start"] != "" {
		start, e := parseDuration(p.attrs["start"])
		if e != nil || start.Sign() != 0 {
			return nil, ErrUnsupported
		}
	}
	duration, err := parseDuration(root.attrs["mediaPresentationDuration"])
	if root.attrs["mediaPresentationDuration"] == "" {
		duration, err = parseDuration(p.attrs["duration"])
	}
	if err != nil || duration.Sign() <= 0 {
		return nil, ErrUnsupported
	}
	if p.attrs["duration"] != "" {
		d, e := parseDuration(p.attrs["duration"])
		if e != nil || d.Cmp(duration) != 0 {
			return nil, ErrUnsupported
		}
	}
	base, err := dashBase(source, root)
	if err != nil {
		return nil, err
	}
	base, err = dashBase(base, p)
	if err != nil {
		return nil, err
	}
	parent, err := dashTemplate(nil, p)
	if err != nil {
		return nil, err
	}
	var video, audio []dashRepresentation
	ids := map[string]bool{}
	totalURI := 0
	for _, a := range p.childrenNamed("AdaptationSet") {
		ab, e := dashBase(base, a)
		if e != nil {
			return nil, e
		}
		at, e := dashTemplate(parent, a)
		if e != nil {
			return nil, e
		}
		reps := a.childrenNamed("Representation")
		if len(reps) == 0 {
			return nil, ErrUnsupported
		}
		for _, rep := range reps {
			rb, e := dashBase(ab, rep)
			if e != nil {
				return nil, e
			}
			rt, e := dashTemplate(at, rep)
			if e != nil {
				return nil, e
			}
			r, e := dashRepresentationPlan(a, rep, rt, rb, duration)
			if e != nil {
				return nil, e
			}
			if ids[r.id] {
				return nil, ErrUnsupported
			}
			ids[r.id] = true
			for _, uri := range r.objects {
				totalURI += len(uri)
			}
			if totalURI > 2<<20 || len(ids) > 64 {
				return nil, ErrLimit
			}
			if r.kind == "video" {
				video = append(video, r)
			} else {
				audio = append(audio, r)
			}
		}
	}
	if len(video) == 0 && len(audio) == 0 {
		return nil, ErrUnsupported
	}
	// Audio-only/video-only use one empty partner; otherwise each choice explicitly
	// identifies both representations, including alternate audio languages.
	if len(video) == 0 {
		video = append(video, dashRepresentation{})
	}
	if len(audio) == 0 {
		audio = append(audio, dashRepresentation{})
	}
	if len(video)*len(audio) > 64 {
		return nil, ErrLimit
	}
	durationSummary, _ := duration.Float64()
	var plans []Plan
	for _, v := range video {
		for _, a := range audio {
			plan := Plan{SourceDigest: hash(body), PlaylistDigest: hash(body), PlaylistURL: source, Duration: durationSummary}
			plan.Variant = Variant{ID: hash([]byte(v.id + "\x00" + a.id)), Bandwidth: v.bandwidth + a.bandwidth, Resolution: v.resolution, Codecs: strings.Trim(v.codecs+","+a.codecs, ",")}
			if a.id != "" {
				plan.Variant.Resolution += " audio " + a.language
			}
			for _, r := range []dashRepresentation{v, a} {
				if r.id != "" {
					plan.Segments = append(plan.Segments, r.objects...)
					plan.Tracks = append(plan.Tracks, Track{Kind: r.kind, End: len(plan.Segments)})
				}
			}
			if len(plan.Segments) > MaxObjects {
				return nil, ErrLimit
			}
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

func dashObjects(t *dashNode, source, id string, bandwidth int64, duration *big.Rat) ([]string, error) {
	if t == nil || t.attrs["initialization"] == "" || t.attrs["media"] == "" {
		return nil, ErrUnsupported
	}
	scale, err := dashInt(t.attrs["timescale"], 1)
	if err != nil || scale == 0 || scale > 1e9 {
		return nil, ErrUnsupported
	}
	offset, err := dashInt(t.attrs["presentationTimeOffset"], 0)
	if err != nil || offset != 0 {
		return nil, ErrUnsupported
	}
	number, err := dashInt(t.attrs["startNumber"], 1)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(source)
	if err != nil {
		return nil, ErrUnsupported
	}
	init, err := dashURL(base, t.attrs["initialization"], id, bandwidth, number, 0, true)
	if err != nil {
		return nil, err
	}
	result := []string{init}
	end, aligned := dashEndTick(duration, scale)
	if !aligned {
		return nil, ErrUnsupported
	}
	var times []int64
	timeline, err := t.one("SegmentTimeline")
	if err != nil {
		return nil, err
	}
	if timeline == nil {
		d, e := dashInt(t.attrs["duration"], 0)
		if e != nil || d == 0 || end%d != 0 {
			return nil, ErrUnsupported
		}
		for time := int64(0); time < end; time += d {
			if len(times) >= MaxObjects-1 {
				return nil, ErrLimit
			}
			times = append(times, time)
		}
	} else {
		if t.attrs["duration"] != "" {
			return nil, ErrUnsupported
		}
		entries := timeline.childrenNamed("S")
		if len(entries) == 0 {
			return nil, ErrUnsupported
		}
		current := int64(0)
		for i, s := range entries {
			start, e := dashInt(s.attrs["t"], current)
			if e != nil || start != current || start >= end {
				return nil, ErrUnsupported
			}
			d, e := dashInt(s.attrs["d"], 0)
			if e != nil || d == 0 {
				return nil, ErrUnsupported
			}
			repeat := int64(0)
			if s.attrs["r"] == "-1" {
				boundary := end
				if i+1 < len(entries) {
					boundary, e = dashInt(entries[i+1].attrs["t"], -1)
					if e != nil || boundary <= current || (boundary-current)%d != 0 {
						return nil, ErrUnsupported
					}
				}
				repeat = (boundary - current - 1) / d
			} else {
				repeat, e = dashInt(s.attrs["r"], 0)
				if e != nil {
					return nil, e
				}
			}
			if repeat >= MaxObjects || int64(len(times))+repeat+1 >= MaxObjects {
				return nil, ErrLimit
			}
			for range repeat + 1 {
				if current >= end {
					return nil, ErrUnsupported
				}
				times = append(times, current)
				if d > 1<<53-current {
					return nil, ErrLimit
				}
				current += d
			}
		}
		if current != end {
			return nil, ErrUnsupported
		}
	}
	seen := map[string]bool{init: true}
	for i, time := range times {
		uri, e := dashURL(base, t.attrs["media"], id, bandwidth, number+int64(i), time, false)
		if e != nil {
			return nil, e
		}
		if seen[uri] {
			return nil, ErrUnsupported
		}
		seen[uri] = true
		result = append(result, uri)
	}
	return result, nil
}

var dashFormat = regexp.MustCompile(`^(Number|Time|Bandwidth)(?:%0([1-9])d)?$`)

func dashURL(base *url.URL, template, id string, bandwidth, number, time int64, initialization bool) (string, error) {
	var out strings.Builder
	dynamic := false
	for template != "" {
		before, after, ok := strings.Cut(template, "$")
		out.WriteString(before)
		if !ok {
			break
		}
		if strings.HasPrefix(after, "$") {
			out.WriteByte('$')
			template = after[1:]
			continue
		}
		token, rest, closed := strings.Cut(after, "$")
		if !closed {
			return "", ErrUnsupported
		}
		template = rest
		if token == "RepresentationID" {
			out.WriteString(id)
			continue
		}
		parts := dashFormat.FindStringSubmatch(token)
		if parts == nil {
			return "", ErrUnsupported
		}
		value := bandwidth
		if parts[1] == "Number" {
			value = number
			dynamic = true
		}
		if parts[1] == "Time" {
			value = time
			dynamic = true
		}
		if initialization && dynamic {
			return "", ErrUnsupported
		}
		width := 0
		if parts[2] != "" {
			width = int(parts[2][0] - '0')
		}
		_, _ = fmt.Fprintf(&out, "%0*d", width, value)
	}
	if !initialization && !dynamic {
		return "", ErrUnsupported
	}
	return resolve(base, out.String())
}
