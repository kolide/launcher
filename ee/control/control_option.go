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
	}
}

// WithGetterSetter sets the key/value getset for control data
func WithGetterSetter(getset types.GetterSetter) Option {
	return func(c *ControlService) {
		c.getset = getset
	}
}
