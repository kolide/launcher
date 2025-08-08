//go:build !windows
// +build !windows

package multislogger

import (
	"io"
)

func SystemSlogger() (*MultiSlogger, io.Closer, error) {
	ms := defaultSystemSlogger()
	// Ensure the dedup engine is stopped when the closer is closed
	closer := closerFunc(func() error {
		ms.Stop()
		return nil
	})
	return ms, closer, nil
}
