// +build windows

package ptycmd

import (
	"github.com/pkg/errors"
)

// NewCmd creates a new command attached to a pty
func NewCmd(command string, argv []string, options ...Option) (*Cmd, error) {
	return nil, errors.New("Not supported on windows")
}
