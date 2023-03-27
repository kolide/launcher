package flags

import "time"

type flagValueOverride interface {
	Value() any
	Start(key FlagKey, value any, duration time.Duration, expiredCallback func(key FlagKey))
}

// Override represents a key-value override and manages the duration for which it is active.
type Override struct {
	key   FlagKey
	value any
	timer *time.Timer
}

// Value returns the value associated with the override
func (o *Override) Value() any {
	return o.value
}

func (o *Override) Start(key FlagKey, value any, duration time.Duration, expiredCallback func(key FlagKey)) {
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
