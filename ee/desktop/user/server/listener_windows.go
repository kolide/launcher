//go:build windows
// +build windows

package server

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func listener(socketPath string) (net.Listener, error) {
	return winio.ListenPipe(socketPath, nil)
}
