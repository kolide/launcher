package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_timebucket(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		// <= 1s returns 1s
		{
			name:     "0 seconds",
			input:    0 * time.Second,
			expected: 1 * time.Second,
		},
		{
			name:     "500ms",
			input:    500 * time.Millisecond,
			expected: 1 * time.Second,
		},
		{
			name:     "1 second",
			input:    1 * time.Second,
			expected: 1 * time.Second,
		},
		// <= 2s returns 2s
		{
			name:     "1.5 seconds",
			input:    1500 * time.Millisecond,
			expected: 1 * time.Second, // truncates to 1s
		},
		{
			name:     "2 seconds",
			input:    2 * time.Second,
			expected: 2 * time.Second,
		},
		// <= 3s returns 3s
		{
			name:     "2.5 seconds",
			input:    2500 * time.Millisecond,
			expected: 2 * time.Second, // truncates to 2s
		},
		{
			name:     "3 seconds",
			input:    3 * time.Second,
			expected: 3 * time.Second,
		},
		// <= 4s returns 4s
		{
			name:     "3.5 seconds",
			input:    3500 * time.Millisecond,
			expected: 3 * time.Second, // truncates to 3s
		},
		{
			name:     "4 seconds",
			input:    4 * time.Second,
			expected: 4 * time.Second,
		},
		// 5-7s returns 6s
		{
			name:     "5 seconds",
			input:    5 * time.Second,
			expected: 6 * time.Second,
		},
		{
			name:     "6 seconds",
			input:    6 * time.Second,
			expected: 6 * time.Second,
		},
		{
			name:     "7 seconds",
			input:    7 * time.Second,
			expected: 6 * time.Second,
		},
		// 8-10s returns 9s
		{
			name:     "8 seconds",
			input:    8 * time.Second,
			expected: 9 * time.Second,
		},
		{
			name:     "9 seconds",
			input:    9 * time.Second,
			expected: 9 * time.Second,
		},
		{
			name:     "10 seconds",
			input:    10 * time.Second,
			expected: 9 * time.Second,
		},
		// 11-13s returns 12s
		{
			name:     "11 seconds",
			input:    11 * time.Second,
			expected: 12 * time.Second,
		},
		{
			name:     "12 seconds",
			input:    12 * time.Second,
			expected: 12 * time.Second,
		},
		{
			name:     "13 seconds",
			input:    13 * time.Second,
			expected: 12 * time.Second,
		},
		// More chunks
		{
			name:     "14 seconds",
			input:    14 * time.Second,
			expected: 15 * time.Second,
		},
		{
			name:     "17 seconds",
			input:    17 * time.Second,
			expected: 18 * time.Second,
		},
		{
			name:     "20 seconds",
			input:    20 * time.Second,
			expected: 21 * time.Second,
		},
		{
			name:     "59 seconds",
			input:    59 * time.Second,
			expected: 60 * time.Second,
		},
		{
			name:     "60 seconds",
			input:    60 * time.Second,
			expected: 60 * time.Second,
		},
		// After 1 minute, round to nearest minute
		{
			name:     "61 seconds",
			input:    61 * time.Second,
			expected: 1 * time.Minute,
		},
		{
			name:     "89 seconds",
			input:    89 * time.Second,
			expected: 1 * time.Minute,
		},
		{
			name:     "90 seconds",
			input:    90 * time.Second,
			expected: 2 * time.Minute,
		},
		{
			name:     "119 seconds",
			input:    119 * time.Second,
			expected: 2 * time.Minute,
		},
		{
			name:     "120 seconds",
			input:    120 * time.Second,
			expected: 2 * time.Minute,
		},
		{
			name:     "150 seconds",
			input:    150 * time.Second,
			expected: 3 * time.Minute,
		},
		{
			name:     "179 seconds",
			input:    179 * time.Second,
			expected: 3 * time.Minute,
		},
		{
			name:     "180 seconds",
			input:    180 * time.Second,
			expected: 3 * time.Minute,
		},
		{
			name:     "5 minutes",
			input:    5 * time.Minute,
			expected: 5 * time.Minute,
		},
		{
			name:     "5 minutes 29 seconds",
			input:    5*time.Minute + 29*time.Second,
			expected: 5 * time.Minute,
		},
		{
			name:     "5 minutes 30 seconds",
			input:    5*time.Minute + 30*time.Second,
			expected: 6 * time.Minute,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := timebucket(tt.input)
			require.Equal(t, tt.expected, actual)
		})
	}
}
