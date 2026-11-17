//go:build windows

package localfs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateWindowsState(t *testing.T) {
	dir := t.TempDir()
	if err := Protect(dir, true); err != nil {
		t.Fatal(err)
	}
	if err := CheckProtected(dir, true); err != nil {
		t.Fatal(err)
	}
	// A new child can have the elevated token's default owner while inheriting
	// the directory's user-only DACL, exactly as SQLite's WAL and SHM files do.
	path := filepath.Join(dir, "sidecar")
	if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckProtected(path, false); err != nil {
		t.Fatal(err)
	}
	if err := Protect(path, false); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if !owner.Equals(user.User.Sid) {
		t.Fatal("protection did not assign current-user ownership")
	}
	if err = CheckProtected(path, false); err != nil {
		t.Fatal(err)
	}
	// An ACL granting Everyone access must be rejected by read-only status.
	broad, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := broad.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	if err = CheckProtected(path, false); err == nil {
		t.Fatal("accepted public ACL")
	}
}
