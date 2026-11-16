package main

import (
	"testing"
)

func TestAggregate(t *testing.T) {
	for _, s := range []string{"", `{}`, `{"quality":{"result":"success"}}`, `{"quality":{"result":"success"},"native":{"result":"skipped"}}`, `{"quality":{"result":"cancelled"},"native":{"result":"success"}}`, `{"quality":{"result":"failure"},"native":{"result":"success"}}`, `{"quality":{"result":"success"},"native":{"result":"success"},"extra":{"result":"success"}}`} {
		if e := aggregate(s); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	if e := aggregate(`{"quality":{"result":"success"},"native":{"result":"success"}}`); e != nil {
		t.Fatal(e)
	}
}

func TestToolVersionMatching(t *testing.T) {
	for _, sample := range []struct {
		output string
		valid  bool
	}{{"v3.0.0", true}, {"tool v3.0.0 built with go1.27.4", true}, {"v3.0.0-beta.1", false}, {"v3.0.01", false}, {"v3.0.10", false}, {"not installed", false}} {
		if hasVersion(sample.output, "v3.0.0") != sample.valid {
			t.Fatalf("incorrect pin validation for %q", sample.output)
		}
	}
}
