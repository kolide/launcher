package flags

import (
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/flags/mocks"
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewDurationFlagValue(log.NewNopLogger(), keys.ControlRequestInterval, tt.options...)
			require.NotNil(t, d)

			val := d.get(tt.controlServerValue)
			assert.Equal(t, tt.expectedValue, val)
		})
	}
}
