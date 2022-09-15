//go:build darwin || linux
// +build darwin linux

package runtime

func dialContext(pid int) func(_ context.Context, _, _ string) (net.Conn, error) {
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", desktop.DesktopSocketPath(pid))
	}
}
