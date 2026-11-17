//go:build !windows && !darwin

package localfs

import (
	"golang.org/x/sys/unix"
	"os"
)

func createOwnershipAttribute(f *os.File, token string) error {
	return unix.Fsetxattr(int(f.Fd()), ownershipAttribute(), []byte(token), unix.XATTR_CREATE)
}
