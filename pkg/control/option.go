package control

import (
	"time"

	"github.com/go-kit/kit/log"
)

type Option func(*Client)

func WithLogger(logger log.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

func WithGetShellsInterval(i time.Duration) Option {
	return func(c *Client) {
		c.getShellsInterval = i
	}
}

func WithDisableTLS() Option {
	return func(c *Client) {
		c.disableTLS = true
	}
}
