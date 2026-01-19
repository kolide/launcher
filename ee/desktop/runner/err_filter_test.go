package runner

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrFilter_filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		matchStrings []string
		err          error
		expected     slog.Level
	}{
		{
			name:         "error matches first match string",
			matchStrings: []string{"signal: killed", "no mapping"},
			err:          errors.New("process received signal: killed"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "error matches second match string",
			matchStrings: []string{"signal: killed", "no mapping"},
			err:          errors.New("no mapping between account names"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "error does not match any match string",
			matchStrings: []string{"signal: killed", "no mapping"},
			err:          errors.New("some other error occurred"),
			expected:     slog.LevelError,
		},
		{
			name:         "case insensitive matching - lowercase error",
			matchStrings: []string{"SIGNAL: KILLED"},
			err:          errors.New("process received signal: killed"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "case insensitive matching - uppercase error",
			matchStrings: []string{"signal: killed"},
			err:          errors.New("process received SIGNAL: KILLED"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "substring matching",
			matchStrings: []string{"signal: killed"},
			err:          errors.New("error: process received signal: killed by system"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "empty match strings",
			matchStrings: []string{},
			err:          errors.New("any error"),
			expected:     slog.LevelError,
		},
		{
			name:         "nil match strings",
			matchStrings: nil,
			err:          errors.New("any error"),
			expected:     slog.LevelError,
		},
		{
			name:         "multiple match strings - first matches",
			matchStrings: []string{"signal: killed", "no mapping", "insufficient resources"},
			err:          errors.New("process received signal: killed"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "multiple match strings - last matches",
			matchStrings: []string{"signal: killed", "no mapping", "insufficient resources"},
			err:          errors.New("insufficient resources available"),
			expected:     slog.LevelWarn,
		},
		{
			name:         "error with empty message",
			matchStrings: []string{"signal: killed"},
			err:          errors.New(""),
			expected:     slog.LevelError,
		},
		{
			name:         "real world example - no mapping error",
			matchStrings: []string{"no mapping between account names and security ids was done"},
			err:          errors.New("no mapping between account names and security ids was done"),
			expected:     slog.LevelWarn,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filter := &errFilter{
				matchStrings: tt.matchStrings,
			}

			result := filter.filter(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrFilter_filter_nilError(t *testing.T) {
	t.Parallel()

	filter := &errFilter{
		matchStrings: []string{"signal: killed"},
	}

	// This should panic or handle nil gracefully - let's test what happens
	require.Panics(t, func() {
		filter.filter(nil)
	}, "filter should panic on nil error")
}
