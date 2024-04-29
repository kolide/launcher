//go:build !windows
// +build !windows

package eventlog

import "errors"

type Writer struct {
}

func NewWriter(name string) (*Writer, error) {
	return nil, errors.New("windows only")
}
