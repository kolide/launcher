//go:build !windows

package multislogger

import (
	"io"
)

func SystemSlogger() (*MultiSlogger, io.Closer, error) {
	return defaultSystemSlogger(), io.NopCloser(nil), nil
}
