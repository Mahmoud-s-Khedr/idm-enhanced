//go:build !windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

func SocketPath() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		var e error
		base, e = os.UserCacheDir()
		if e != nil {
			return "", e
		}
	}
	dir := filepath.Join(base, "idm-enhanced")
	if e := os.Mkdir(dir, 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return "", e
	}
	info, e := os.Lstat(dir)
	if e != nil {
		return "", e
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || int(stat.Uid) != os.Getuid() {
		return "", errors.New("IPC directory must be owned by this user with mode 0700 and must not be a symlink")
	}
	path := filepath.Join(dir, "desktop.sock")
	if len(path) > 100 {
		return "", errors.New("IPC socket path too long; set XDG_RUNTIME_DIR to a short private directory")
	}
	return path, nil
}
func Listen() (net.Listener, error) {
	path, e := SocketPath()
	if e != nil {
		return nil, e
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if e != nil {
		return nil, fmt.Errorf("IPC socket busy; close the other app, or remove a stale socket after verifying no app is running: %w", e)
	}
	if e = os.Chmod(path, 0600); e != nil {
		_ = l.Close()
		return nil, e
	}
	return l, nil
}

func dial(ctx context.Context) (net.Conn, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}
