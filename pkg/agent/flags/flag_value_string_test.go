package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlagValueString(t *testing.T) {
	t.Parallel()

	// mockOverride := mocks.NewFlagValueOverride(t)
	// mockOverride.On("Value").Return(7 * time.Second)

	tests := []struct {
		name               string
		options            []stringFlagValueOption
		controlServerValue []byte
		expected           string
	}{
		{
			name: "nil",
		},
		{
			name:     "default only",
			options:  []stringFlagValueOption{WithDefaultString("DEFAULT")},
			expected: "DEFAULT",
		},
		{
			name:               "control server no options",
			controlServerValue: []byte("control-server-says-this"),
			expected:           "control-server-says-this",
		},
		{
			name:               "control server with default",
			options:            []stringFlagValueOption{WithDefaultString("DEFAULT")},
			controlServerValue: []byte("control-server-says-this"),
			expected:           "control-server-says-this",
		},
		{
			name: "default with sanitizer",
			options: []stringFlagValueOption{WithDefaultString("DEFAULT"), WithSanitizer(func(value string) string {
				return "SANITIZED DEFAULT"
			})},
			expected: "SANITIZED DEFAULT",
		},
		{
			name: "default and control, with sanitizer",
			options: []stringFlagValueOption{WithDefaultString("DEFAULT"), WithSanitizer(func(value string) string {
				return "SANITIZED control-server-says-this"
			})},
			controlServerValue: []byte("control-server-says-this"),
			expected:           "SANITIZED control-server-says-this",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewStringFlagValue(tt.options...)
			assert.Equal(t, tt.expected, s.get(tt.controlServerValue))
		})
	}
}
