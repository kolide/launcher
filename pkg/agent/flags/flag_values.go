package flags

// Alias which allows any type
type AnyFlagValues = flagValues[any]

type flagValues[T any] struct {
	flags map[FlagKey]T
}

// NewFlagValues returns a new typed flagValues struct.
func NewFlagValues[T any]() *flagValues[T] {
	f := &flagValues[T]{
		flags: make(map[FlagKey]T),
	}

	return f
}

// Set sets the value for a FlagKey.
func (f *flagValues[T]) Set(key FlagKey, value T) {
	f.flags[key] = value
}

// Get retrieves the value for a FlagKey.
func (f *flagValues[T]) Get(key FlagKey) (T, bool) {
	value, exists := f.flags[key]
	return value, exists
}
