package acceleratecontrolconsumer

import (
	"strings"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/stretchr/testify/require"
)

func TestAccelerateControlConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		data             string
		expectedInterval time.Duration
		expectedDuration time.Duration
		wantErr          bool
	}{
		{
			name:             "happy path",
			data:             `{"interval": 123, "duration": 456}`,
			expectedInterval: 123 * time.Second,
			expectedDuration: 456 * time.Second,
		},
		{
			name:    "bad json",
			data:    `ABC`,
			wantErr: true,
		},
		{
			name:    "bad interval",
			data:    `{"interval": "ABC", "duration": 10}`,
			wantErr: true,
		},
		{
			name:    "bad duration",
			data:    `{"interval": 10, "duration": "ABC"}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSack := mocks.NewKnapsack(t)

			if !tt.wantErr {
				mockSack.On("SetControlRequestIntervalOverride", tt.expectedInterval, tt.expectedDuration)
			}

			c := New(mockSack)
			err := c.Do(strings.NewReader(tt.data))

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
