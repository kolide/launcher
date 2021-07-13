package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchOsqueryEmojiHandling(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{
			in:  `\xF0\x9F\x9A\xB2`,
			out: `🚲`,
		},
		{
			in:  `\xFNOCANDOBUDDY`,
			out: `\xFNOCANDOBUDDY`,
		},
		{
			in:  `a normal string`,
			out: `a normal string`,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Input: %s", tt.in), func(t *testing.T) {
			assert.Equal(t, tt.out, patchOsqueryEmojiHandling(tt.in))
		})
	}
}

func TestPatchOsqueryEmojiHandlingArray(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  []string
		out []string
	}{
		{
			in:  []string{},
			out: []string{},
		},
		{
			in:  []string{`\xFNOCANDOBUDDY`, `a normal string`, `\xF0\x9F\x9A\xB2`},
			out: []string{`\xFNOCANDOBUDDY`, `a normal string`, `🚲`},
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Input: %s", tt.in), func(t *testing.T) {
			require.Equal(t, tt.out, patchOsqueryEmojiHandlingArray(tt.in))
		})
	}
}
