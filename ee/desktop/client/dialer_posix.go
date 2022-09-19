//go:build darwin || linux
// +build darwin linux

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
