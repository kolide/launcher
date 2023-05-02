//go:build !windows
// +build !windows

package client

import (
	"context"
	"net"
)

func dialContext(socketPath string) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
}
