package control

import (
	"time"
)

type Option func(*ControlService)

// WithUpdateInterval sets the interval on which the control service will request updates from k2
func WithRequestInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.requestInterval = interval // TODO: can come from knapsack? Not needed this func?
		c.requestTicker.Reset(interval)
	}
}

// WithMinAcceleartionInterval sets the minimum interval between updates during request interval acceleration
func WithMinAcclerationInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.minAccelerationInterval = interval
	}
}
