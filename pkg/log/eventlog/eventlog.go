// +build !windows

package eventlog

import "github.com/go-kit/kit/log"

// New creates a log.Logger that writes to the Windows Event Log.
// The Logger formats event data using logfmt.
func New(w *Writer) log.Logger {
	panic("Windows Only")
}
