package ptycmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"github.com/kr/pty"
	"github.com/pkg/errors"
)

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

// NewCmd creates a new command attached to a pty
func NewCmd(command string, argv []string, options ...Option) (*Cmd, error) {
	// create the command
	cmd := exec.Command(command, argv...)

	// open a pty
	pty, tty, err := pty.Open()
	if err != nil {
		return nil, errors.Wrap(err, "opening pty")
	}
	defer tty.Close()

	// attach the stdin/stdout/stderr of the pty's tty to the cmd
	cmd.Stdout = tty
	cmd.Stdin = tty
	cmd.Stderr = tty

	// start the cmd, closing the PTY if there's an error
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrapf(err, "starting command  `%s`", command)
	}

	ptyClosed := make(chan struct{})

	// create the command with default options
	ptyCmd := &Cmd{
		command:      command,
		argv:         argv,
		cmd:          cmd,
		pty:          pty,
		ptyClosed:    ptyClosed,
		closeSignal:  syscall.SIGINT,
		closeTimeout: 10 * time.Second,
	}

	// apply the given options using the functional opts pattern
	for _, option := range options {
		option(ptyCmd)
	}

	// if the process is closed, close the pty
	go func() {
		ptyCmd.cmd.Wait()
		ptyCmd.pty.Close()
		close(ptyCmd.ptyClosed)
	}()

	return ptyCmd, nil
}

// Read reads into the specified buffer from the pty
func (c *Cmd) Read(p []byte) (n int, err error) {
	return c.pty.Read(p)
}

// Write writes from the specified buffer into the pty
func (c *Cmd) Write(p []byte) (n int, err error) {
	return c.pty.Write(p)
}

// Close closes the underlying process and the pty
func (c *Cmd) Close() error {
	// if the process exists, send the close signal
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Signal(c.closeSignal)
	}
	for {
		select {
		// either it was closed
		case <-c.ptyClosed:
			return nil

		// or timeout
		case <-time.After(c.closeTimeout):
			c.cmd.Process.Signal(syscall.SIGKILL)
			return nil
		}
	}
}

// Title returns a title for the TTY window
func (c *Cmd) Title() string {
	hostName, err := os.Hostname()
	if err != nil {
		hostName = "remote-host"
	}
	return fmt.Sprintf("%s@%s", c.command, hostName)
}

// Resize resizes the terminal of the process
func (c *Cmd) Resize(width int, height int) error {
	window := struct {
		row uint16
		col uint16
		x   uint16
		y   uint16
	}{
		uint16(height),
		uint16(width),
		0,
		0,
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		c.pty.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&window)),
	)
	if errno != 0 {
		return errno
	}
	return nil
}
