// +build windows

package ptycmd

import "errors"

// NewCmd creates a new command attached to a pty
func NewCmd(command string, argv []string, options ...Option) (*Cmd, error) {
	return nil, errors.New("Not supported on windows")
}

func (c *Cmd) Close() error {
	return errors.New("Not supported on windows")
}

func (c *Cmd) Read(p []byte) (n int, err error) {
	return c.pty.Read(p)
}

// Write writes from the specified buffer into the pty
func (c *Cmd) Write(p []byte) (n int, err error) {
	return c.pty.Write(p)
}

func (c *Cmd) Resize(width int, height int) error {
	return nil
}

func (c *Cmd) Title() string {
	return ""
}
