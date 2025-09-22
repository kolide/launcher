package flags

import (
	"testing"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/flags/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloat64FlagValue(t *testing.T) {
	t.Parallel()

	mockOverride := mocks.NewFlagValueOverride(t)
	mockOverride.On("Value").Return(1.2)

	tests := []struct {
		name               string
		options            []float64Option
		controlServerValue []byte
		expectedValue      float64
	}{
		{
			name:          "zero value",
			expectedValue: 0.0,
		},
		{
			name:          "default only",
			options:       []float64Option{WithFloat64ValueDefault(1.0)},
			expectedValue: 1.0,
		},
		{
			name:               "control server no options",
			controlServerValue: []byte("0.6667"),
			expectedValue:      0.6667,
		},
		{
			name:               "control server with default",
			options:            []float64Option{WithFloat64ValueDefault(0.75)},
			controlServerValue: []byte("0.5"),
			expectedValue:      0.5,
		},
		{
			name:               "bad control server value",
			options:            []float64Option{WithFloat64ValueDefault(0.75)},
			controlServerValue: []byte("NOT A NUMBER"),
			expectedValue:      0.75,
		},
		{
			name:               "control server with override",
			options:            []float64Option{WithFloat64ValueDefault(5.0), WithFloat64ValueOverride(mockOverride)},
			controlServerValue: []byte("3.4"),
			expectedValue:      1.2,
		},
		{
			name:               "control server with default, min -- needs clamp",
			options:            []float64Option{WithFloat64ValueDefault(1.0), WithFloat64ValueMin(0.0)},
			controlServerValue: []byte("-1.0"),
			expectedValue:      0.0,
		},
		{
			name:               "control server with default, max -- needs clamp",
			options:            []float64Option{WithFloat64ValueDefault(0.8), WithFloat64ValueMax(1.0)},
			controlServerValue: []byte("3.0"),
			expectedValue:      1.0,
		},
		{
			name:               "control server with default, min -- does not need clamp",
			options:            []float64Option{WithFloat64ValueDefault(1.0), WithFloat64ValueMin(0.0)},
			controlServerValue: []byte("0.25"),
			expectedValue:      0.25,
		},
		{
			name:               "control server with default, max -- does not need clamp",
			options:            []float64Option{WithFloat64ValueDefault(0.8), WithFloat64ValueMax(1.0)},
			controlServerValue: []byte("0.7"),
			expectedValue:      0.7,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewFloat64FlagValue(multislogger.NewNopLogger(), keys.ControlRequestInterval, tt.options...)
			require.NotNil(t, d)

			val := d.get(tt.controlServerValue)
			assert.Equal(t, tt.expectedValue, val)
		})
	}
}
