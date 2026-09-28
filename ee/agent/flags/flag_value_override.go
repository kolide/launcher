package flags

import (
	"time"

	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
)

// FlagValueOverride is an interface for an override which can be active for a duration of
// time, with a special-case value, until it expires.
//
//mockery:generate: true
//mockery:filename: flag_value_override.go
type FlagValueOverride interface {
	// Value gets the override value.
	Value() any
}

// Override represents a key-value override and holds the timer for its expiration
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
