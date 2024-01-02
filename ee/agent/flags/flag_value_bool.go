package flags

type boolOption func(*boolFlagValue)

func WithDefaultBool(defaultVal bool) boolOption {
	return func(b *boolFlagValue) {
		b.defaultVal = defaultVal
	}
}

func WithBoolOverride(override FlagValueOverride) boolOption {
	return func(f *boolFlagValue) {
		f.override = override
	}
}

type boolFlagValue struct {
	defaultVal bool
	override   FlagValueOverride
}

func NewBoolFlagValue(opts ...boolOption) *boolFlagValue {
	b := &boolFlagValue{}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

func (b *boolFlagValue) get(controlServerValue []byte) bool {
	boolValue := b.defaultVal
	if controlServerValue != nil {
		boolValue = bytesToBool(controlServerValue)
	}

	if b.override != nil && b.override.Value() != nil {
		// An override was provided, if it's valid let it take precedence
		value, ok := b.override.Value().(bool)
		if ok {
			boolValue = value
		}
	}

	return boolValue
}

func boolToBytes(enabled bool) []byte {
	if enabled {
		return []byte("enabled")
	}
	return []byte("")
}

func bytesToBool(controlServerValue []byte) bool {
	var booleanValue bool
	if string(controlServerValue) == "enabled" {
		return true
	}
	return booleanValue
}

func BoolToString(enabled bool) string {
	return string(boolToBytes(enabled))
}

func StringToBool(controlServerValue string) bool {
	return bytesToBool([]byte(controlServerValue))
}
