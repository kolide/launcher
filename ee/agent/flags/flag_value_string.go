package flags

type stringFlagValueOption func(*stringFlagValue)

func WithOverrideString(override FlagValueOverride) stringFlagValueOption {
	return func(s *stringFlagValue) {
		s.override = override
	}
}

func WithDefaultString(defaultVal string) stringFlagValueOption {
	return func(s *stringFlagValue) {
		s.defaultVal = defaultVal
	}
}

func WithSanitizer(sanitizer func(value string) string) stringFlagValueOption {
	return func(s *stringFlagValue) {
		s.sanitizer = sanitizer
	}
}

type stringFlagValue struct {
	defaultVal string
	sanitizer  func(value string) string
	override   FlagValueOverride
}

func NewStringFlagValue(opts ...stringFlagValueOption) *stringFlagValue {
	s := &stringFlagValue{}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *stringFlagValue) get(controlServerValue []byte) string {
	stringValue := s.defaultVal
	if controlServerValue != nil {
		stringValue = string(controlServerValue)
	}

	if s.override != nil && s.override.Value() != nil {
		// An override was provided, if it's valid let it take precedence
		value, ok := s.override.Value().(string)
		if ok {
			stringValue = value
		}
	}

	// Run the string through a sanitizer, if one was provided
	if s.sanitizer != nil {
		stringValue = s.sanitizer(stringValue)
	}

	return stringValue
}
