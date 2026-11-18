package ipc

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func SocketPath() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\idm-enhanced-` + user.User.Sid.String(), nil
}
func Listen() (net.Listener, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + user.User.Sid.String() + ")", InputBufferSize: 65536, OutputBufferSize: 65536})
}
func dial(ctx context.Context) (net.Conn, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	return winio.DialPipeContext(ctx, path)
}
