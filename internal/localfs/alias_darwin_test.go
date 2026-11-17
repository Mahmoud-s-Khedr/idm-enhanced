package localfs

import (
	"os"
	"testing"
)

func TestSystemTemporaryAliases(t *testing.T) {
	for _, path := range []string{"/var", "/tmp"} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 && !SystemAlias(path) {
			t.Errorf("rejected macOS system alias %s", path)
		}
	}
	if SystemAlias(t.TempDir()) {
		t.Fatal("accepted arbitrary alias")
	}
}
