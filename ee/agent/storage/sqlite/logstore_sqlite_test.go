package agentsqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/stretchr/testify/require"
)

func TestAppendValue(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	s, err := OpenRW(context.TODO(), testRootDir, WatchdogLogStore)
	require.NoError(t, err, "creating test store")

	flagKey := []byte(keys.UpdateChannel.String())
	flagVal := []byte("beta")

	startTime := time.Now().Unix()
	logEntry := `{"time":"2024-05-13T18:29:31.7829101Z", "msg":"testmsg"}`
	require.NoError(t, s.AppendValue(startTime, []byte(logEntry)), "expected no error appending value row")

	returnedVal, err := s.Get(flagKey)
	require.NoError(t, err, "expected no error getting value")
	require.Equal(t, flagVal, returnedVal, "flag value mismatch")

	require.NoError(t, s.Close())
}
