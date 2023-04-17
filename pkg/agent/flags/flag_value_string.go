package flags

type stringFlagValueOption func(*stringFlagValue)

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

	// Run the string through a sanitizer, if one was provided
	if s.sanitizer != nil {
		stringValue = s.sanitizer(stringValue)
	}
	return stringValue
}
