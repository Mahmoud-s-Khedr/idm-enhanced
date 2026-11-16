// Package protocol owns the M0 ping contract and native-message framing.
package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

const Version = 1
const MaxMessage = 64 * 1024
const Timeout = 5 * time.Second

var requestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Message struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Type    string `json:"type"`
	Error   string `json:"error,omitempty"`
}

func (m Message) Validate() error {
	if m.Version != Version {
		return errors.New("unsupported protocol version")
	}
	if !requestID.MatchString(m.ID) {
		return errors.New("invalid request ID")
	}
	switch m.Type {
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
	if response.Type != "ack" {
		return errors.New("expected acknowledgement")
	}
	return nil
}
