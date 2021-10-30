//go:build !windows
// +build !windows

package eventlog

import "github.com/go-kit/kit/log"

func New(w *Writer) log.Logger {
	panic("Windows Only")
}
