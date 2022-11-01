//go:build !windows
// +build !windows

package socket

import (
	"context"
	"net"
)

func Dial(socketPath string) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
}

func Listen(socketPath string) (net.Listener, error) {
	return net.Listen("unix", socketPath)
}
