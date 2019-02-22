// +build !windows

package eventlog

// NewWriter creates and installs a windows.Handle that writes to the Windows Event log.
// Use the Close method to close the handle.
func NewWriter(name string) (*Writer, error) {
	panic("windows only")
}
