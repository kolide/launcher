package flags

import (
	"time"
)

// FlagValueConstraints returns a map of FlagKey->flagValueConstraints
func FlagValueConstraints() map[FlagKey]flagValueConstraint {
	constraints := make(map[FlagKey]flagValueConstraint)

	// Below is a list of integer FlagKeys and their constraints
	constraints[ControlRequestInterval] = flagValueConstraint{
		min: int64(5 * time.Second),
		max: int64(10 * time.Minute),
	}

	return constraints
}
