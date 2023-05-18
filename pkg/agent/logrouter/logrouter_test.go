package logrouter

import (
	"encoding/json"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestReplay(t *testing.T) {
	t.Parallel()

	msg := "I am a fancy log message " + ulid.New()

	lr, err := New(log.NewNopLogger())
	require.NoError(t, err)

	level.Debug(lr.Logger()).Log("msg", msg)

	// Ensure it got logged
	recent, err := lr.GetRecentDebugLogs()
	require.NoError(t, err)
	recentAsBytes, err := json.Marshal(recent)
	require.NoError(t, err)
	require.Contains(t, string(recentAsBytes), msg, "destination internal debug ring")

	tests := []struct {
		name   string
		level  level.Option
		expect require.ComparisonAssertionFunc
	}{
		{
			name:   "info",
			level:  level.AllowInfo(),
			expect: require.NotContains,
		},
		{
			name:   "debug",
			level:  level.AllowDebug(),
			expect: require.Contains,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf threadsafebuffer.ThreadSafeBuffer
			logger := log.NewJSONLogger(&buf)
			logger = level.NewFilter(logger, tt.level)

			require.NoError(t, lr.Replay(logger))

			tt.expect(t, buf.String(), msg)

		})
	}
}
