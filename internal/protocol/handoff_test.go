package protocol

import "testing"

func TestDownloadBoundary(t *testing.T) {
	good := Message{Version: 1, ID: "handoff", Type: "download", URL: "https://example.invalid/file?token=private", Filename: "file.bin"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequest(good); err != nil {
		t.Fatal(err)
	}
	accepted := Message{Version: 1, ID: good.ID, Type: "accepted", JobID: "job-1"}
	if err := Match(good, accepted); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"blob:https://example.invalid/a", "file:///tmp/a", "https://u:p@example.invalid/a", "https://example.invalid/a#part", "https:///missing", "%"} {
		r := good
		r.URL = source
		if r.Validate() == nil {
			t.Fatal(source)
		}
	}
	for _, name := range []string{"", "../file", "a/b", "a\\b", "a:", "CON.txt", "LPT1", "COM9", "NUL", "a\x00b", ".", "..", "x ", "x."} {
		r := good
		r.Filename = name
		if r.Validate() == nil {
			t.Fatal(name)
		}
	}
	for _, r := range []Message{
		{Version: 1, ID: "x", Type: "ping", Filename: "a"},
		{Version: 1, ID: "x", Type: "ack", JobID: "job"},
		{Version: 1, ID: "x", Type: "accepted"},
		{Version: 1, ID: "x", Type: "download", Error: "error"},
	} {
		if r.Validate() == nil {
			t.Fatal(r)
		}
	}
	if ValidateRequest(accepted) == nil {
		t.Fatal("response treated as request")
	}
	good.Type = "ping"
	if Match(good, accepted) == nil {
		t.Fatal("accepted ping")
	}
}
