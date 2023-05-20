package logrouter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
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

func TestAddTargets(t *testing.T) {
	t.Parallel()

	msg1 := "I am an early log message " + ulid.New()
	msg2 := "I am a late log message " + ulid.New()

	lr, err := New(log.NewNopLogger())
	require.NoError(t, err)

	level.Info(lr.Logger()).Log("msg", msg1)

	{
		// Since there's no store added, the recent logs should be empty
		recent, err := lr.GetRecentLogs()
		require.NoError(t, err)
		require.Nil(t, recent, "recent logs should be empty")
	}

	store := inmemory.NewStore(log.NewNopLogger())
	require.NoError(t, lr.AddPersistStore(store))
	require.Error(t, lr.AddPersistStore(store), "does not allow second store")

	debugfile := filepath.Join(t.TempDir(), "debug.json")
	require.NoError(t, lr.AddDebugFile(debugfile))

	// Log something _after_ we've joined the additional targets. This ensures that we replay
	level.Info(lr.Logger()).Log("msg", msg2)

	t.Run("memory ring", func(t *testing.T) {
		t.Parallel()

		recent, err := lr.GetRecentDebugLogs()
		require.NoError(t, err)
		recentAsBytes, err := json.Marshal(recent)
		require.NoError(t, err)
		require.Contains(t, string(recentAsBytes), msg1, "msg1")
		require.Contains(t, string(recentAsBytes), msg2, "msg2")
	})

	t.Run("persisted ring store", func(t *testing.T) {
		t.Parallel()

		recent, err := lr.GetRecentLogs()
		require.NoError(t, err)
		require.NotNil(t, recent)
		recentAsBytes, err := json.Marshal(recent)
		require.NoError(t, err)
		require.Contains(t, string(recentAsBytes), msg1, "msg1")
		require.Contains(t, string(recentAsBytes), msg2, "msg2")
	})

	t.Run("debug file on disk", func(t *testing.T) {
		t.Parallel()

		debugjsonContents, err := os.ReadFile(debugfile)
		require.NoError(t, err)
		require.Contains(t, string(debugjsonContents), msg1, "msg1")
		require.Contains(t, string(debugjsonContents), msg2, "msg2")
	})
}

func TestLogRouting(t *testing.T) {
	t.Parallel()

	var systemLogBuffer threadsafebuffer.ThreadSafeBuffer
	systemlogger := log.NewJSONLogger(&systemLogBuffer)
	systemlogger = level.NewFilter(systemlogger, level.AllowInfo())

	lr, err := New(systemlogger)
	require.NoError(t, err)

	store := inmemory.NewStore(log.NewNopLogger())
	require.NoError(t, lr.AddPersistStore(store))

	systemDebugMsg := "system debug " + ulid.New()
	systemInfoMsg := "system info " + ulid.New()
	internalDebugMsg := "internal debug " + ulid.New()
	internalInfoMsg := "internal info " + ulid.New()

	level.Debug(lr.SystemLogger()).Log("msg", systemDebugMsg)
	level.Info(lr.SystemLogger()).Log("msg", systemInfoMsg)
	level.Debug(lr.Logger()).Log("msg", internalDebugMsg)
	level.Info(lr.Logger()).Log("msg", internalInfoMsg)

	tests := []struct {
		name              string
		msg               string
		expectSystem      require.ComparisonAssertionFunc
		expectPersistRing require.ComparisonAssertionFunc
	}{
		{
			name:              "system debug",
			msg:               systemDebugMsg,
			expectSystem:      require.NotContains,
			expectPersistRing: require.NotContains,
		},
		{
			name:              "system info",
			msg:               systemInfoMsg,
			expectSystem:      require.Contains,
			expectPersistRing: require.Contains,
		},
		{
			name:              "internal debug",
			msg:               internalDebugMsg,
			expectSystem:      require.NotContains,
			expectPersistRing: require.NotContains,
		},
		{
			name:              "internal info",
			msg:               internalInfoMsg,
			expectSystem:      require.NotContains,
			expectPersistRing: require.Contains,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// We always expect in the debug ring
			recentDebugLogs, err := lr.GetRecentDebugLogs()
			require.NoError(t, err)
			recentDebugLogsAsBytes, err := json.Marshal(recentDebugLogs)
			require.NoError(t, err)
			require.Contains(t, string(recentDebugLogsAsBytes), tt.msg, "destination internal debug ring")

			// May or may not be in the system log
			tt.expectSystem(t, systemLogBuffer.String(), tt.msg, "destination system log")

			recentLogs, err := lr.GetRecentLogs()
			require.NoError(t, err)
			recentLogsAsBytes, err := json.Marshal(recentLogs)
			require.NoError(t, err)

			tt.expectPersistRing(t, string(recentLogsAsBytes), tt.msg, "destination internal persist ring")
		})
	}

}
