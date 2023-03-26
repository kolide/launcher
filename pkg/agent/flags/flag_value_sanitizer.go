package flags

// flagValueConstraint represents the constraining limits on a flag value.
type flagValueConstraint struct {
	min int64
	max int64
}

type flagValueSanitizer struct {
	constraints map[FlagKey]flagValueConstraint
}

func NewFlagValueSanitizer(constraints map[FlagKey]flagValueConstraint) *flagValueSanitizer {
	s := &flagValueSanitizer{
		constraints: constraints,
	}

	return s
}

// Sanitize returns a sanitized (clamped) value for a flag, if constraints for
// the flag value have been provided. If undefined, the original value is returned.
func (s *flagValueSanitizer) Sanitize(key FlagKey, value int64) int64 {
	c, ok := s.constraints[key]
	if ok {
		return clampValue(value, c.min, c.max)
	}
	return value
}

// clampValue returns a value that is clamped to be within the range defined by min and max.
func clampValue(value int64, min, max int64) int64 {
	switch {
	case value < min:
		return min
	case value > max:
		return max
	default:
		return value
	}
}
