//go:build darwin || linux
// +build darwin linux

package runtime

import (
	"context"
	"net"

	"github.com/kolide/launcher/ee/desktop"
)

func dialContext(pid int) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", desktop.DesktopSocketPath(pid))
	}
}
