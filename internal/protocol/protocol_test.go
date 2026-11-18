package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSharedFixtures(t *testing.T) {
	b, e := os.ReadFile("../../testdata/protocol/messages.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name    string
		Valid   bool
		Message json.RawMessage
	}
	if e = json.Unmarshal(b, &cases); e != nil {
		t.Fatal(e)
	}
	if len(cases) == 0 {
		t.Fatal("empty fixture suite")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			_, e := Decode(c.Message)
			if (e == nil) != c.Valid {
				t.Fatalf("valid=%v error=%v", c.Valid, e)
			}
		})
	}
}
func TestFrames(t *testing.T) {
	m := Message{Version: Version, ID: "round-trip", Type: "ping"}
	var b bytes.Buffer
	if e := Write(&b, m); e != nil {
		t.Fatal(e)
	}
	frame := append([]byte(nil), b.Bytes()...)
	got, e := Read(&b)
	if e != nil || got != m {
		t.Fatalf("%+v %v", got, e)
	}
	for _, n := range []int{0, 1, 3, 4, len(frame) - 1} {
		if _, e = Read(bytes.NewReader(frame[:n])); e == nil {
			t.Fatalf("accepted partial frame %d", n)
		}
	}
	for _, n := range []uint32{0, MaxMessage + 1} {
		var header [4]byte
		binary.LittleEndian.PutUint32(header[:], n)
		if _, e = Read(bytes.NewReader(header[:])); e == nil {
			t.Fatal("accepted invalid length")
		}
	}
}
func TestInvalidJSON(t *testing.T) {
	for _, s := range []string{"{", `{} {}`, strings.Repeat("x", MaxMessage+1), `{"version":1,"id":"a","type":"error","error":"` + strings.Repeat("x", 513) + `"}`} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatal("accepted invalid message")
		}
	}
}

type brokenWriter struct{ short bool }

func (w brokenWriter) Write(b []byte) (int, error) {
	if w.short {
		return len(b) - 1, nil
	}
	return 0, io.ErrClosedPipe
}
func TestWriteFailure(t *testing.T) {
	m := Message{Version: Version, ID: "x", Type: "ping"}
	for _, w := range []io.Writer{brokenWriter{}, brokenWriter{short: true}} {
		if e := Write(w, m); e == nil {
			t.Fatal("ignored writer failure")
		}
	}
	if e := Write(io.Discard, Message{}); e == nil {
		t.Fatal("accepted invalid message")
	}
}
func TestAcknowledgements(t *testing.T) {
	ping := Message{Version: Version, ID: "x", Type: "ping"}
	ack, e := Ack(ping)
	if e != nil || Match(ping, ack) != nil {
		t.Fatal("valid ack failed")
	}
	for _, m := range []Message{{}, ack} {
		if _, e = Ack(m); e == nil {
			t.Fatal("accepted non-ping")
		}
	}
	for _, m := range []Message{{}, ping, {Version: Version, ID: "other", Type: "ack"}, {Version: Version, ID: "x", Type: "error", Error: "closed"}} {
		if e = Match(ping, m); e == nil {
			t.Fatal("accepted wrong reply")
		}
	}
}

func TestHLSModeIsBoundedAndOnlyValidOnDownloadRequests(t *testing.T) {
	request := Message{Version: 1, ID: "media", Type: "download", URL: "https://example.invalid/list.m3u8?token=private", Filename: "list.m3u8", Mode: "hls"}
	var frame bytes.Buffer
	if err := Write(&frame, request); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&frame)
	if err != nil || got != request {
		t.Fatal(got, err)
	}
	for _, value := range []Message{{Version: 1, ID: "media", Type: "ping", Mode: "hls"}, {Version: 1, ID: "media", Type: "accepted", JobID: "job", Mode: "hls"}, {Version: 1, ID: "media", Type: "download", URL: request.URL, Filename: request.Filename, Mode: "unknown"}} {
		if err := value.Validate(); err == nil {
			t.Fatal("unexpected media mode accepted")
		}
	}
}
