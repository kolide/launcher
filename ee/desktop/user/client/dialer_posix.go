//go:build !windows
// +build !windows

package client

import (
	"context"
	"net"
)

func dialContext(socketPath string) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socketPath)
	}
}
