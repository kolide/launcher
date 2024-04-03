//go:build !windows
// +build !windows

package main

import (
	"io"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func systemSlogger() (*multislogger.MultiSlogger, io.Closer, error) {
	return defaultSystemSlogger(), io.NopCloser(nil), nil
}
