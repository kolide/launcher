//go:build windows
// +build windows

package socket

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func Dial(socketPath string) (net.Conn, error) {
	return winio.DialPipe(socketPath, nil)
}

func Listen(socketPath string) (net.Listener, error) {
	return winio.ListenPipe(socketPath, nil)
}
