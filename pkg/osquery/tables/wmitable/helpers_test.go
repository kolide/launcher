package wmitable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnlyAllowedCharacters(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		input     string
		assertion assert.BoolAssertionFunc
	}{
		{"hello", assert.True},
		{"Hello", assert.True},
		{"hello world", assert.False},
		{"hello;", assert.False},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tt.assertion(t, onlyAllowedCharacters(tt.input))
		})
	}

}
