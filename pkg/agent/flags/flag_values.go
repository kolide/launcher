package flags

type FlagValues struct {
	flags map[FlagKey]any
}

func NewFlagValues() *FlagValues {
	f := &FlagValues{
		flags: make(map[FlagKey]any),
	}

	return f
}

// Set sets the value for a FlagKey.
func (f *FlagValues) Set(key FlagKey, value any) {
	f.flags[key] = value
}

// Get retrieves the value for a FlagKey.
func (f *FlagValues) Get(key FlagKey) (any, bool) {
	value, exists := f.flags[key]
	return value, exists
}
