package ptycmd

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Option is a configuration option for a Cmd
type Option func(*Cmd)

// Cmd is a shelled out command and an attached pty
type Cmd struct {
	// the command that is being relayed
	command string

	// args passed to the command
	argv []string

	// the external command struct
	cmd *exec.Cmd

	// the pseudoterminal attached to the command
	pty *os.File

	// channel to signal closing the pty
	ptyClosed chan struct{}

	// signal to close process
	closeSignal syscall.Signal

	// time to wait to close process
	closeTimeout time.Duration
}

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
