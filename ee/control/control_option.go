package control

import (
	"github.com/kolide/launcher/pkg/agent/types"
)

type Option func(*ControlService)

// WithStore sets the key/value store for control data
func WithStore(store types.GetterSetter) Option {
	return func(c *ControlService) {
		c.store = store
	}
}
