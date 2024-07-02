package agentsqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAppendAndIterateValues(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	s, err := OpenRW(context.TODO(), testRootDir, WatchdogLogStore)
	require.NoError(t, err, "creating test store")

	startTime := time.Now()
	expectedLogCount := 5
	for i := 0; i < expectedLogCount; i++ {
		currTime := startTime.Add(time.Duration(i) * time.Minute)
		logEntry := fmt.Sprintf(`{"time":"%s", "msg":"testMessage%d"}`, currTime.Format(time.RFC3339), i)
		require.NoError(t, s.AppendValue(currTime.Unix(), []byte(logEntry)), "expected no error appending value row")
	}

	logsSeen := 0
	err = s.ForEach(func(rowid, timestamp int64, v []byte) error {
		logRecord := make(map[string]any)

		require.NoError(t, json.Unmarshal(v, &logRecord), "expected to be able to unmarshal row value")
		expectedTime := startTime.Add(time.Duration(logsSeen) * time.Minute)
		require.Equal(t, expectedTime.Unix(), timestamp, "expected log timestamp to match")

		logsSeen++
		return nil
	})

	require.NoError(t, err, "expected no error iterating over new logs")
	require.Equal(t, expectedLogCount, logsSeen, "did not see expected count of logs during iteration")

	require.NoError(t, s.Close())
}
