// +build !windows

package ptycmd

import (
	"syscall"
	"time"
)

// Option is a configuration option for a Cmd
type Option func(*Cmd)

// WithCloseSignal lets you specify the signal to send to the
// underlying process on close
func WithCloseSignal(signal syscall.Signal) Option {
	return func(c *Cmd) {
		c.closeSignal = signal
	}
}

// WithCloseTimeout lets you specify how long to wait for the
// underlying process to terminate before sending a SIGKILL
func WithCloseTimeout(timeout time.Duration) Option {
	return func(c *Cmd) {
		c.closeTimeout = timeout
	}
}
