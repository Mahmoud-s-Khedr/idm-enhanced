package localfs

import (
	"errors"
	"io"
	"os"
)

const ownershipStream = ":idm-enhanced-owner"

// NTFS named streams remain attached to the file across exclusive hard links.
func TagOwnership(f *os.File, token string) (err error) {
	tag, err := os.OpenFile(f.Name()+ownershipStream, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tag.Close()) }()
	if _, err = tag.WriteString(token); err != nil {
		return err
	}
	return tag.Sync()
}
func VerifyOwnership(f *os.File, token string) (err error) {
	tag, err := os.Open(f.Name() + ownershipStream)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tag.Close()) }()
	data, err := io.ReadAll(io.LimitReader(tag, 65))
	if err != nil {
		return err
	}
	if token == "" || string(data) != token {
		return errors.New("file allocation ownership mismatch")
	}
	return nil
}
