package webtty

import (
	"encoding/json"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

// Option is an option for WebTTY.
type Option func(*WebTTY) error

// WithPermitWrite sets a WebTTY to accept input from the TTY.
func WithPermitWrite() Option {
	return func(wt *WebTTY) error {
		wt.permitWrite = true
		return nil
	}
}

// WithFixedColumns sets a fixed width to the TTY.
func WithFixedColumns(columns int) Option {
	return func(wt *WebTTY) error {
		wt.columns = columns
		return nil
	}
}

// WithFixedRows sets a fixed height to the TTY.
func WithFixedRows(rows int) Option {
	return func(wt *WebTTY) error {
		wt.rows = rows
		return nil
	}
}

// WithTitle sets the default window title of the session
func WithTitle(title []byte) Option {
	return func(wt *WebTTY) error {
		wt.title = title
		return nil
	}
}

// WithReconnect enables reconnection on the TTY side.
func WithReconnect(timeInSeconds int) Option {
	return func(wt *WebTTY) error {
		wt.reconnect = timeInSeconds
		return nil
	}
}

// WithTTYPreferences sets an optional configuration of TTY.
func WithTTYPreferences(preferences interface{}) Option {
	return func(wt *WebTTY) error {
		prefs, err := json.Marshal(preferences)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal preferences as JSON")
		}
		wt.ttyPreferences = prefs
		return nil
	}
}

// WithLogger sets the logger to use
func WithLogger(logger log.Logger) Option {
	return func(wt *WebTTY) error {
		wt.logger = logger
		return nil
	}
}
