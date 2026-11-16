//go:build !linux

package ipc

import (
	"errors"
	"net"
)

func SocketPath() (string, error) {
	return "", errors.New("live browser IPC is Linux-only in M0; Windows/macOS support is scheduled for M5")
}
func Listen() (net.Listener, error) { _, e := SocketPath(); return nil, e }
