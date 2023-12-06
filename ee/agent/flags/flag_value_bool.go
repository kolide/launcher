package flags

type boolOption func(*boolFlagValue)

func WithDefaultBool(defaultVal bool) boolOption {
	return func(b *boolFlagValue) {
		b.defaultVal = defaultVal
	}
}

type boolFlagValue struct {
	defaultVal bool
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
