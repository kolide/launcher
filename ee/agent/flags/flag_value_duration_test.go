package flags

import (
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/flags/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagValueDuration(t *testing.T) {
	t.Parallel()

	mockOverride := mocks.NewFlagValueOverride(t)
	mockOverride.On("Value").Return(7 * time.Second)

	tests := []struct {
		name               string
		options            []durationOption
		controlServerValue []byte
		expectedValue      any
	}{
		{
			name:          "zero value",
			expectedValue: time.Duration(0),
		},
		{
			name:          "default only",
			options:       []durationOption{WithDefault(5 * time.Second)},
			expectedValue: 5 * time.Second,
		},
		{
			name:               "control server no options",
			controlServerValue: []byte("120000000000"),
			expectedValue:      2 * time.Minute,
		},
		{
			name:               "control server with default",
			options:            []durationOption{WithDefault(5 * time.Second)},
			controlServerValue: []byte("120000000000"),
			expectedValue:      2 * time.Minute,
		},
		{
			name:               "bad control server value",
			options:            []durationOption{WithDefault(5 * time.Second)},
			controlServerValue: []byte("NOT A NUMBER"),
			expectedValue:      5 * time.Second,
		},
		{
			name:               "control server with override",
			options:            []durationOption{WithDefault(5 * time.Second), WithOverride(mockOverride)},
			controlServerValue: []byte("120000"),
			expectedValue:      7 * time.Second,
		},
		{
			name:               "control server with default, min",
			options:            []durationOption{WithDefault(20 * time.Second), WithMin(10 * time.Second)},
			controlServerValue: []byte("5000000000"),
			expectedValue:      10 * time.Second,
		},
		{
			name:               "control server with default, max",
			options:            []durationOption{WithDefault(20 * time.Second), WithMax(25 * time.Second)},
			controlServerValue: []byte("30000000000"),
			expectedValue:      25 * time.Second,
		},
		// New tests for duration string support
		{
			name:               "duration string seconds",
			controlServerValue: []byte("4s"),
			expectedValue:      4 * time.Second,
		},
		{
			name:               "duration string minutes",
			controlServerValue: []byte("10m"),
			expectedValue:      10 * time.Minute,
		},
		{
			name:               "duration string hours",
			controlServerValue: []byte("2h"),
			expectedValue:      2 * time.Hour,
		},
		{
			name:               "duration string complex",
			controlServerValue: []byte("1h30m45s"),
			expectedValue:      1*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:               "duration string with default fallback",
			options:            []durationOption{WithDefault(5 * time.Second)},
			controlServerValue: []byte("30s"),
			expectedValue:      30 * time.Second,
		},
		{
			name:               "legacy nanoseconds still work",
			options:            []durationOption{WithDefault(5 * time.Second)},
			controlServerValue: []byte("300000000000"), // 5 minutes in nanoseconds
			expectedValue:      5 * time.Minute,
		},
		{
			name:               "invalid duration string falls back to default",
			options:            []durationOption{WithDefault(5 * time.Second)},
			controlServerValue: []byte("invalid-duration"),
			expectedValue:      5 * time.Second,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewDurationFlagValue(multislogger.NewNopLogger(), keys.ControlRequestInterval, tt.options...)
			require.NotNil(t, d)

			val := d.get(tt.controlServerValue)
			assert.Equal(t, tt.expectedValue, val)
		})
	}
}
