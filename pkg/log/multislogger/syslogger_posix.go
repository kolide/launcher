//go:build !windows
// +build !windows

package multislogger

import (
	"io"
)

func systemSlogger() (*MultiSlogger, io.Closer, error) {
	return defaultSystemSlogger(), io.NopCloser(nil), nil
}
