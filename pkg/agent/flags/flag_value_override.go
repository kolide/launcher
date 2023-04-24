package flags

import (
	"time"

	"github.com/kolide/launcher/pkg/agent/flags/keys"
)

// FlagValueOverride is an interface for an override which can be active for a duration of
// time, with a special-case value, until it expires.
type FlagValueOverride interface {
	// Value gets the override value.
	Value() any
	// Start begins or resets the duration for which the override is active. The expiredCallback
	// func will be invoked when the override expires.
	Start(key keys.FlagKey, value any, duration time.Duration, expiredCallback func(key keys.FlagKey))
}

// Override represents a key-value override and manages the duration for which it is active.
type Override struct {
	key   keys.FlagKey
	value any
	timer *time.Timer
}

// Value returns the value associated with the override
func (o *Override) Value() any {
	if o == nil {
		return nil
	}

	return o.value
}

func (o *Override) Start(key keys.FlagKey, value any, duration time.Duration, expiredCallback func(key keys.FlagKey)) {
	if o == nil {
		return
	}

	// Stop existing timer, if necessary
	if o.timer != nil {
		// To ensure the channel is empty after a call to Stop, check the
		// return value and drain the channel.
		if !o.timer.Stop() {
			<-o.timer.C
		}
	}

	// Update the key value (if key already exists, it shouldn't change)
	o.key = key
	o.value = value

	// Invoke the expiration callback after duration has passed
	o.timer = time.AfterFunc(duration, func() {
		expiredCallback(o.key)
	})
}
