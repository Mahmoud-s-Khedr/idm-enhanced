package localfs

import "os"

// SystemAlias permits only macOS's documented system temporary-directory aliases.
func SystemAlias(path string) bool {
	if path != "/var" && path != "/tmp" {
		return false
	}
	target, err := os.Readlink(path)
	return err == nil && target == "/private"+path
}
