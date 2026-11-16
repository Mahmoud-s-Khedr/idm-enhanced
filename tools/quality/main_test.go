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
