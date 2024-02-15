package autoupdate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeUpdateChannel(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		name            string
		channel         string
		expectedChannel string
	}{
		{
			name:            "default",
			expectedChannel: Stable.String(),
		},
		{
			name:            "alpha",
			channel:         Alpha.String(),
			expectedChannel: Alpha.String(),
		},
		{
			name:            "beta",
			channel:         Beta.String(),
			expectedChannel: Beta.String(),
		},
		{
			name:            "nightly",
			channel:         Nightly.String(),
			expectedChannel: Nightly.String(),
		},
		{
			name:            "invalid",
			channel:         "not-a-real-channel",
			expectedChannel: Stable.String(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expectedChannel, SanitizeUpdateChannel(tt.channel))
		})
	}
}
