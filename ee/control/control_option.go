package control

import (
	"time"
)

type Option func(*ControlService)

func WithRequestInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.requestInterval = interval
	}
}
