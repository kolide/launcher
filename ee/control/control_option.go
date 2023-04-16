package control

import (
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
)

type Option func(*ControlService)

// WithUpdateInterval sets the interval on which the control service will request updates from k2
func WithRequestInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.requestInterval = interval
		// c.requestTicker.Reset(interval)
	}
}

// WithStore sets the key/value store for control data
func WithStore(store types.GetterSetter) Option {
	return func(c *ControlService) {
		c.store = store
	}
}

// WithMinAcceleartionInterval sets the minimum interval between updates during request interval acceleration
func WithMinAcclerationInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.minAccelerationInterval = interval
	}
}
