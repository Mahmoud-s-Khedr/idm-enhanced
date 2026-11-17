//go:build !windows

package localfs

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
)

func ownershipAttribute() string {
	if runtime.GOOS == "darwin" {
		return "org.idm-enhanced.owner"
	}
	return "user.idm-enhanced.owner"
}

// TagOwnership binds a fresh random allocation token to the inode, independently
// of its reusable native ID. Filesystems without extended attributes fail closed.
func TagOwnership(f *os.File, token string) error {
	return unix.Fsetxattr(int(f.Fd()), ownershipAttribute(), []byte(token), unix.XATTR_CREATE)
}
func VerifyOwnership(f *os.File, token string) error {
	var buffer [64]byte
	n, err := unix.Fgetxattr(int(f.Fd()), ownershipAttribute(), buffer[:])
	if err != nil {
		return err
	}
	if token == "" || string(buffer[:n]) != token {
		return errors.New("file allocation ownership mismatch")
	}
	return nil
}
