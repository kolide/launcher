package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_isChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "stable", input: "stable", expected: true},
		{name: "beta", input: "beta", expected: true},
		{name: "nightly", input: "nightly", expected: true},
		{name: "alpha", input: "alpha", expected: true},
		{name: "version", input: "5.12.1", expected: false},
		{name: "empty", input: "", expected: false},
		{name: "uppercase channel", input: "Stable", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, isChannel(tt.input))
		})
	}
}
