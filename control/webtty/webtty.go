package webtty

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/pkg/errors"
)

var (
	// ErrPTYClosed indicates the function has exited by the pty
	ErrPTYClosed = errors.New("PTY closed")

	// ErrTTYClosed is returned when the tty connection is closed.
	ErrTTYClosed = errors.New("TTY closed")
)

const (
	// Protocol defines the name of this protocol,
	// which is supposed to be used to the subprotocol of Websocket streams.
	Protocol = "webtty"

	// INCOMING MESSAGE TYPES

	// UnknownInput is an unknown message type, maybe sent by a bug
	UnknownInput = '0'
	// Input is user input typically from a keyboard
	Input = '1'
	// Ping to the server
	Ping = '2'
	// Resize is to notify that the TTY size has been changed
	Resize = '3'

	// OUTGOING MESSAGE TYPES

	// UnknownOutput is an unknown message type, maybe sent by a bug
	UnknownOutput = '0'
	// Output is normal output to the terminal
	Output = '1'
	// Pong to the browser
	Pong = '2'
	// SetTitle of the terminal
	SetTitle = '3'
	// SetPreferences of the terminal
	SetPreferences = '4'
	// SetReconnect is to signal the terminal to reconnect
	SetReconnect = '5'
)

// WebTTY bridges a PTY pty and its PTY tty.
// To support text-based streams and side channel commands such as
// terminal resizing, WebTTY uses an original protocol.
type WebTTY struct {
	// the attached TTY
	tty TTY

	// the PTY that the TTY is proxied to
	pty PTY

	// the title of the TTY
	title []byte

	// allow writes to this TTY
	permitWrite bool

	// TTY width
	columns int

	// TTY height
	rows int

	// how many seconds to reconnect after
	reconnect int

	// JSON preferences for the TTY
	ttyPreferences []byte

	// size of buffer for messages
	bufferSize int

	// lock to ensure threadsafe
	writeMutex sync.Mutex
}

// TTY represents a TTY connection, typically a websocket
type TTY io.ReadWriteCloser

// PTY represents a PTY pty, typically it's a local command.
type PTY interface {
	io.ReadWriteCloser

	// Get the title for the TTY
	Title() string

	// Resize the attached PTY
	Resize(columns int, rows int) error
}

// New creates a new instance of WebTTY,
// given a connection to the TTY and a PTY to connect it to.
func New(tty TTY, pty PTY, options ...Option) (*WebTTY, error) {
	wt := &WebTTY{
		tty:         tty,
		pty:         pty,
		title:       []byte(pty.Title()),
		permitWrite: false,
		columns:     0,
		rows:        0,

		bufferSize: 2048,
	}

	for _, option := range options {
		option(wt)
	}

	return wt, nil
}

// Run starts the main process of the WebTTY.
// This method blocks until the context is canceled.
// Note that the tty and pty are left intact even
// after the context is canceled. Closing them is caller's
// responsibility.
// If the connection to one end gets closed, returns ErrPTYClosed or ErrTTYCLosed.
func (wt *WebTTY) Run(ctx context.Context) error {
	// send message to TTY to initialize
	// a title, reconnect time, and preferences
	err := wt.sendInitializeMessage()
	if err != nil {
		return errors.Wrapf(err, "failed to send initializing message")
	}

	// make a channel to return errors over
	errs := make(chan error, 2)

	// spawn goroutine to relay PTY messages to the TTY
	go func() {
		errs <- func() error {
			buffer := make([]byte, wt.bufferSize)
			for {
				n, err := wt.pty.Read(buffer)
				if err != nil {
					return ErrPTYClosed
				}

				err = wt.relayToTTY(buffer[:n])
				if err != nil {
					return err
				}
			}
		}()
	}()

	// spawn goroutine to relay TTY messages to the PTY
	go func() {
		errs <- func() error {
			buffer := make([]byte, wt.bufferSize)
			for {
				n, err := wt.tty.Read(buffer)
				if err != nil {
					log.Println(err)
					return ErrTTYClosed
				}

				err = wt.relayToPTY(buffer[:n])
				if err != nil {
					return err
				}
			}
		}()
	}()

	// wait for the context to be closed, returning any errors
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-errs:
	}

	return err
}

// sendInitializeMessage sends the title, reconnect time,
// and preferences to the TTY on startup
func (wt *WebTTY) sendInitializeMessage() error {
	err := wt.writeTTY(append([]byte{SetTitle}, wt.title...))
	if err != nil {
		return errors.Wrapf(err, "failed to send window title")
	}

	if wt.reconnect > 0 {
		reconnect, _ := json.Marshal(wt.reconnect)
		err := wt.writeTTY(append([]byte{SetReconnect}, reconnect...))
		if err != nil {
			return errors.Wrapf(err, "failed to set reconnect")
		}
	}

	if wt.ttyPreferences != nil {
		err := wt.writeTTY(append([]byte{SetPreferences}, wt.ttyPreferences...))
		if err != nil {
			return errors.Wrapf(err, "failed to set preferences")
		}
	}

	return nil
}

// relayToTTY encodes the message and writes to the TTY
func (wt *WebTTY) relayToTTY(data []byte) error {
	safeMessage := base64.StdEncoding.EncodeToString(data)
	err := wt.writeTTY(append([]byte{Output}, []byte(safeMessage)...))
	if err != nil {
		return errors.Wrapf(err, "failed to send message to tty")
	}

	return nil
}

// writeTTY writes to the TTY in a threadsafe way
func (wt *WebTTY) writeTTY(data []byte) error {
	// lock when writing to not clobber messages
	wt.writeMutex.Lock()
	defer wt.writeMutex.Unlock()

	// write the data to the TTY
	_, err := wt.tty.Write(data)
	if err != nil {
		return errors.Wrapf(err, "failed to write to tty")
	}

	return nil
}

// relayToPTY handles writing different message types from the TTY
// and writing any input to the PTY
func (wt *WebTTY) relayToPTY(data []byte) error {
	// make sure the read yielded data
	if len(data) == 0 {
		return errors.New("unexpected zero length read from tty")
	}

	switch data[0] {
	// handle input
	case Input:
		// check if we can write to the webTTY
		if !wt.permitWrite {
			return nil
		}

		// make sure there's data to send and not an empty input message
		if len(data) < 2 {
			return nil
		}

		// write the data to the pty
		_, err := wt.pty.Write(data[1:])
		if err != nil {
			return errors.Wrapf(err, "failed to write received data to pty")
		}

	// handle a ping by returning a pong
	case Ping:
		err := wt.writeTTY([]byte{Pong})
		if err != nil {
			return errors.Wrapf(err, "failed to return Pong message to tty")
		}

	// handle a resize message
	case Resize:
		// don't set if the payload for resize is empty
		if len(data) < 2 {
			return errors.New("received malformed remote command for terminal resize: empty payload")
		}

		// read the json payload to resize
		var args struct {
			Columns float64
			Rows    float64
		}
		err := json.Unmarshal(data[1:], &args)
		if err != nil {
			return errors.Wrapf(err, "received malformed data for terminal resize")
		}

		wt.pty.Resize(int(args.Columns), int(args.Rows))

	// catch all to handle unknown messages
	default:
		// return errors.Errorf("unknown message type `%c`", data[0])
		log.Printf("unknown message type `%v`", string(data))
	}

	return nil
}
