//go:build darwin || linux
// +build darwin linux

package server

import "net"

func listener(socketPath string) (net.Listener, error) {
	return net.Listen("unix", socketPath)
}
