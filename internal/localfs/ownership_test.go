package localfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllocationOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err = Protect(path, false); err != nil {
		t.Fatal(err)
	}
	if err = VerifyOwnership(f, "original"); err == nil {
		t.Fatal("accepted untagged allocation")
	}
	if err = TagOwnership(f, "original"); err != nil {
		t.Fatal(err)
	}
	if err = VerifyOwnership(f, "original"); err != nil {
		t.Fatal(err)
	}
	if err = VerifyOwnership(f, ""); err == nil {
		t.Fatal("accepted empty token")
	}
	if err = VerifyOwnership(f, "replacement"); err == nil {
		t.Fatal("accepted different allocation")
	}
	if err = TagOwnership(f, "replacement"); err == nil {
		t.Fatal("replaced existing ownership evidence")
	}
	link := path + ".published"
	if err = os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	published, err := os.Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyOwnership(published, "original"); err != nil {
		t.Fatal("publication lost ownership evidence", err)
	}
	if err = published.Close(); err != nil {
		t.Fatal(err)
	}
}
