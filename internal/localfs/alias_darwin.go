package localfs

import (
	"os"
	"path/filepath"
)

// SystemAlias permits only macOS's documented system temporary-directory aliases.
func SystemAlias(path string) bool {
	if path != "/var" && path != "/tmp" {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target) == "/private"+path
}
