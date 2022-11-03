//go:build !windows
// +build !windows

package socket

import (
	"net"
)

func Dial(socketPath string) (net.Conn, error) {
	return net.Dial("unix", socketPath)
}

func Listen(socketPath string) (net.Listener, error) {
	return net.Listen("unix", socketPath)
}
