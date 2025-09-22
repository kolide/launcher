package flags

import (
	"testing"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/flags/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntFlagValue(t *testing.T) {
	t.Parallel()

	mockOverride := mocks.NewFlagValueOverride(t)
	mockOverride.On("Value").Return(1)

	tests := []struct {
		name               string
		options            []intOption
		controlServerValue []byte
		expectedValue      int
	}{
		{
			name:          "zero value",
			expectedValue: 0,
		},
		{
			name:          "default only",
			options:       []intOption{WithIntValueDefault(2)},
			expectedValue: 2,
		},
		{
			name:               "control server no options",
			controlServerValue: []byte("3"),
			expectedValue:      3,
		},
		{
			name:               "control server with default",
			options:            []intOption{WithIntValueDefault(3)},
			controlServerValue: []byte("5"),
			expectedValue:      5,
		},
		{
			name:               "bad control server value",
			options:            []intOption{WithIntValueDefault(3)},
			controlServerValue: []byte("NOT A NUMBER"),
			expectedValue:      3,
		},
		{
			name:               "control server with override",
			options:            []intOption{WithIntValueDefault(3), WithIntValueOverride(mockOverride)},
			controlServerValue: []byte("4"),
			expectedValue:      1,
		},
		{
			name:               "control server with default, min -- needs clamp",
			options:            []intOption{WithIntValueDefault(3), WithIntValueMin(0)},
			controlServerValue: []byte("-1"),
			expectedValue:      0,
		},
		{
			name:               "control server with default, max -- needs clamp",
			options:            []intOption{WithIntValueDefault(3), WithIntValueMax(100)},
			controlServerValue: []byte("200"),
			expectedValue:      100,
		},
		{
			name:               "control server with default, min -- does not need clamp",
			options:            []intOption{WithIntValueDefault(3), WithIntValueMin(0)},
			controlServerValue: []byte("1"),
			expectedValue:      1,
		},
		{
			name:               "control server with default, max -- does not need clamp",
			options:            []intOption{WithIntValueDefault(3), WithIntValueMax(100)},
			controlServerValue: []byte("70"),
			expectedValue:      70,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := NewIntFlagValue(multislogger.NewNopLogger(), keys.ControlRequestInterval, tt.options...)
			require.NotNil(t, d)

			val := d.get(tt.controlServerValue)
			assert.Equal(t, tt.expectedValue, val)
		})
	}
}
