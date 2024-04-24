package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_TrivialTruncate(t *testing.T) {
	t.Parallel()

	const maxLen = 5
	var tests = []struct {
		in       string
		expected string
	}{
		{
			in:       "",
			expected: "",
		},
		{
			in:       "1",
			expected: "1",
		},
		{
			in:       "short",
			expected: "short",
		},
		{
			in:       "and this is a long string",
			expected: "and t...",
		},
		{
			in:       "我喜歡吃辣的食物",
			expected: "我\xe5\x96...",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()
		})

		actual := trivialTruncate(tt.in, maxLen)
		require.Equal(t, tt.expected, actual)
	}

}
