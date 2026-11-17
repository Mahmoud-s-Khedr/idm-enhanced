//go:build !windows

package localfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeOperations(t *testing.T) {
	dir := t.TempDir()
	if err := Protect(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := CheckProtected(dir, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "file")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	id, err := Identity(f)
	if err != nil || id == "" {
		t.Fatal(id, err)
	}
	if err = Protect(path, false); err != nil {
		t.Fatal(err)
	}
	if err = CheckProtected(path, false); err != nil {
		t.Fatal(err)
	}
	if err = Lock(f); err != nil {
		t.Fatal(err)
	}
	other, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Lock(other); err == nil {
		t.Fatal("second owner")
	}
	if err = other.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Identity(f); err == nil {
		t.Fatal("closed identity")
	}
	if err = SyncDir(dir); err != nil {
		t.Fatal(err)
	}
	if err = SyncDir(path + "missing"); err == nil {
		t.Fatal("missing sync")
	}
	if err = Protect(path+"missing", false); err == nil {
		t.Fatal("missing protect")
	}
	if err = CheckProtected(path+"missing", false); err == nil {
		t.Fatal("missing check")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err = CheckProtected(path, false); err == nil {
		t.Fatal("unsafe permissions")
	}
	if err = Protect(path, true); err == nil {
		t.Fatal("accepted wrong type")
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err = Protect(link, false); err == nil {
		t.Fatal("accepted symlink")
	}
}
