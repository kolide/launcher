//go:build windows
// +build windows

package client

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialContext(socketPath string) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return winio.DialPipe(socketPath, nil)
	}
}
