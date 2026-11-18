// Package protocol owns the browser handoff contract and native-message framing.
package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const Version = 1
const MaxMessage = 64 * 1024
const Timeout = 5 * time.Second

var requestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Message struct {
	Version  int    `json:"version"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Error    string `json:"error,omitempty"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
	JobID    string `json:"jobId,omitempty"`
}

func (m Message) Validate() error {
	if m.Version != Version {
		return errors.New("unsupported protocol version")
	}
	if !requestID.MatchString(m.ID) {
		return errors.New("invalid request ID")
	}
	if m.Type != "download" && (m.URL != "" || m.Filename != "") {
		return errors.New("unexpected download fields")
	}
	if m.Type != "accepted" && m.JobID != "" {
		return errors.New("unexpected job ID")
	}
	switch m.Type {
	case "download":
		if m.Error != "" {
			return errors.New("unexpected error field")
		}
		return ValidateDownload(m.URL, m.Filename)
	case "accepted":
		if !requestID.MatchString(m.JobID) || m.Error != "" {
			return errors.New("invalid accepted reply")
		}
	case "ping", "ack":
		if m.Error != "" {
			return errors.New("unexpected error field")
		}
	case "error":
		if m.Error == "" || len(m.Error) > 512 {
			return errors.New("invalid error message")
		}
	default:
		return errors.New("unsupported message type")
	}
	return nil
}

// ValidateDownload rejects browser-only URLs, credentials and unsafe portable names.
func ValidateDownload(source, filename string) error {
	u, err := url.Parse(source)
	if err != nil || len(source) > 16384 || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return errors.New("only credential-free HTTP(S) URLs without fragments are supported")
	}
	if filename == "" || len(filename) > 180 || strings.ContainsAny(filename, `/\\:<>"|?*`) || strings.TrimRight(filename, " .") != filename || filename == "." || filename == ".." {
		return errors.New("invalid filename")
	}
	for _, r := range filename {
		if unicode.IsControl(r) {
			return errors.New("invalid filename")
		}
	}
	stem := strings.ToUpper(strings.SplitN(filename, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
		return errors.New("reserved filename")
	}
	return nil
}
func ValidateRequest(m Message) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Type != "ping" && m.Type != "download" {
		return errors.New("expected browser request")
	}
	return nil
}
func Decode(b []byte) (Message, error) {
	var m Message
	if len(b) > MaxMessage {
		return m, errors.New("message exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, errors.New("malformed message")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return m, errors.New("trailing JSON data")
	}
	return m, m.Validate()
}
func Read(r io.Reader) (Message, error) {
	var size [4]byte
	if _, e := io.ReadFull(r, size[:]); e != nil {
		return Message{}, fmt.Errorf("read frame length: %w", e)
	}
	n := binary.LittleEndian.Uint32(size[:])
	if n == 0 || n > MaxMessage {
		return Message{}, errors.New("invalid frame length (maximum 64 KiB)")
	}
	data := make([]byte, n)
	if _, e := io.ReadFull(r, data); e != nil {
		return Message{}, fmt.Errorf("read frame body: %w", e)
	}
	return Decode(data)
}
func Write(w io.Writer, m Message) error {
	if e := m.Validate(); e != nil {
		return e
	}
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	if len(b) > MaxMessage {
		return errors.New("message exceeds 64 KiB")
	}
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(b)))
	for _, part := range [][]byte{size[:], b} {
		n, e := w.Write(part)
		if e != nil {
			return e
		}
		if n != len(part) {
			return io.ErrShortWrite
		}
	}
	return nil
}
func Ack(m Message) (Message, error) {
	if e := m.Validate(); e != nil {
		return Message{}, e
	}
	if m.Type != "ping" {
		return Message{}, errors.New("expected ping")
	}
	return Message{Version: Version, ID: m.ID, Type: "ack"}, nil
}
func Match(request, response Message) error {
	if e := response.Validate(); e != nil {
		return e
	}
	if response.ID != request.ID {
		return errors.New("acknowledgement ID mismatch")
	}
	if response.Type == "error" {
		return errors.New(response.Error)
	}
	expected := "ack"
	if request.Type == "download" {
		expected = "accepted"
	}
	if response.Type != expected {
		return errors.New("expected acknowledgement")
	}
	return nil
}
