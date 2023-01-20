package control

import (
	"time"

	"github.com/kolide/launcher/pkg/agent"
)

type Option func(*ControlService)

// WithUpdateInterval sets the interval on which the control service will request updates from k2
func WithRequestInterval(interval time.Duration) Option {
	return func(c *ControlService) {
		c.requestInterval = interval
	}
}

// WithRetrieverStorer sets the key/value storer for control data
func WithRetrieverStorer(storer agent.RetrieverStorer) Option {
	return func(c *ControlService) {
		c.storer = storer
	}
}
