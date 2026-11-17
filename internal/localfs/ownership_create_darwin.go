package localfs

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func createOwnershipAttribute(f *os.File, token string) error {
	// x/sys v0.49.0's Darwin Fsetxattr ignores flags. Keep descriptor binding
	// and refuse an existing attribute explicitly. The manager exclusively owns
	// this fresh allocation, so there is no second writer between these calls.
	_, err := unix.Fgetxattr(int(f.Fd()), ownershipAttribute(), nil)
	if err == nil {
		return unix.EEXIST
	}
	if !errors.Is(err, unix.ENOATTR) {
		return err
	}
	return unix.Fsetxattr(int(f.Fd()), ownershipAttribute(), []byte(token), 0)
}
