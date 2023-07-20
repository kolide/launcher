package acceleratecontrolconsumer

import (
	"strings"
	"testing"

	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAccelerateControlConsumer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name: "happy path",
			data: `{"interval": "10s", "duration": "10s"}`,
		},
		{
			name:    "bad json",
			data:    `ABC`,
			wantErr: true,
		},
		{
			name:    "no interval",
			data:    `{"duration": "10s"}`,
			wantErr: true,
		},
		{
			name:    "bad interval",
			data:    `{"interval": "ABC", "duration": "10s"}`,
			wantErr: true,
		},
		{
			name:    "no duration",
			data:    `{"interval": "10s"}`,
			wantErr: true,
		},
		{
			name:    "bad duration",
			data:    `{"interval": "10s", "duration": "ABC"}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSack := mocks.NewKnapsack(t)

			if !tt.wantErr {
				mockSack.On("SetControlRequestIntervalOverride", mock.Anything, mock.Anything)
			}

			c := New(mockSack)
			err := c.Update(strings.NewReader(tt.data))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
