package logthicket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogThicket(t *testing.T) {
	t.Parallel()

	rootDirectory := t.TempDir()

	var systemLogBuffer threadsafebuffer.ThreadSafeBuffer
	systemlogger := log.NewJSONLogger(&systemLogBuffer)
	systemlogger = level.NewFilter(systemlogger, level.AllowInfo())

	store := inmemory.NewStore(log.NewNopLogger())

	lt, err := New(systemlogger, rootDirectory, store)
	require.NoError(t, err)

	systemDebugMsg := "system debug " + ulid.New()
	systemInfoMsg := "system info " + ulid.New()
	internalDebugMsg := "internal debug " + ulid.New()
	internalInfoMsg := "internal info " + ulid.New()

	level.Debug(lt.SystemLogger()).Log("msg", systemDebugMsg)
	level.Info(lt.SystemLogger()).Log("msg", systemInfoMsg)
	level.Debug(lt.Logger()).Log("msg", internalDebugMsg)
	level.Info(lt.Logger()).Log("msg", internalInfoMsg)

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

			// We always expect logs in debug.json
			debugjsonContents, err := os.ReadFile(filepath.Join(rootDirectory, "debug.json"))
			require.NoError(t, err)
			assert.Contains(t, string(debugjsonContents), tt.msg, "destination debug.json")

			// We always expect in the debug ring
			recentDebugLogs, err := lt.GetRecentDebugLogs()
			require.NoError(t, err)
			recentDebugLogsAsBytes, err := json.Marshal(recentDebugLogs)
			require.NoError(t, err)
			assert.Contains(t, string(recentDebugLogsAsBytes), tt.msg, "destination internal debug ring")

			tt.expectSystem(t, systemLogBuffer.String(), tt.msg, "destination system log")

			// The goal of these tests is to test the different log functions. In some go world, it feels better
			// to do this by defining functions. But taking that route feels like it creates a lot of
			// duplication around also defining the name. So while these switch feels awkward, ultimately
			// is seems simpler than the alternatives.

			recentLogs, err := lt.GetRecentLogs()
			require.NoError(t, err)
			recentLogsAsBytes, err := json.Marshal(recentLogs)
			require.NoError(t, err)
			switch {
			case strings.HasSuffix(tt.name, " debug"):
				assert.NotContains(t, string(recentLogsAsBytes), tt.msg, "destination internal persist ring")
			case strings.HasSuffix(tt.name, " info"):
				assert.Contains(t, string(recentLogsAsBytes), tt.msg, "destination internal persist ring")
			default:
				require.FailNowf(t, "can't map %s to level", tt.name)
			}

		})
	}
}
