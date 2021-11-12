//go:build windows
// +build windows

package eventlog

import (
	"errors"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/eventlog"
)

// NewWriter creates and installs a windows.Handle that writes to the Windows Event log.
// Use the Close method to close the handle.
func NewWriter(name string) (*Writer, error) {
	h, err := openHandle("", name)
	if err != nil {
		return nil, err
	}
	if err := eventlog.InstallAsEventCreate(name, eventlog.Info); !isAlreadyExists(err) {
		return nil, err
	}
	return &Writer{handle: h}, nil
}

func openHandle(host, source string) (h windows.Handle, err error) {
	if source == "" {
		return h, errors.New("specify event log source")
	}
	var s *uint16
	if host != "" {
		s = syscall.StringToUTF16Ptr(host)
	}
	h, err = windows.RegisterEventSource(s, syscall.StringToUTF16Ptr(source))
	return h, err
}

type Writer struct {
	handle windows.Handle
}

func (w *Writer) Close() error {
	return windows.DeregisterEventSource(w.handle)
}

func (w *Writer) Write(p []byte) (n int, err error) {
	ss := []*uint16{syscall.StringToUTF16Ptr(string(p))}
	// always report as Info. Launcher logs as either info or debug, but the event log does not
	// appear to have a debug level.
	err = windows.ReportEvent(w.handle, windows.EVENTLOG_INFORMATION_TYPE, 0, 1, 0, 1, 0, &ss[0], nil)
	return len(p), err
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "registry key already exists")
}
