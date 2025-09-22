package checkups

import (
	"context"
	"io"
	"testing"

	"github.com/kolide/launcher/ee/performance"
	"github.com/stretchr/testify/require"
)

func Test_checkPerformance(t *testing.T) {
	t.Parallel()
	perf := perfCheckup{}
	err := perf.Run(context.TODO(), io.Discard)
	require.NoError(t, err, "unexpected error encountered gathering performance stats")
	require.Contains(t, perf.data, "stats")
	stats := perf.data["stats"]
	perfStats, ok := stats.(*performance.PerformanceStats)
	require.True(t, ok, "expected stats to be PerformanceStats")
	// Note that we don't check CPU because it is often 0 in CI, resulting in a flaky test
	require.NotEmpty(t, perfStats.Exe, "expected exe to be set on performance stats")
	require.Greater(t, perfStats.Pid, 0, "expected pid to be set")
	require.NotNil(t, perfStats.MemInfo, "expected MemInfo to be set")
	require.Greater(t, perfStats.MemInfo.RSS, uint64(0), "expected RSS to be set")
	require.Greater(t, perfStats.MemInfo.VMS, uint64(0), "expected VMS to be set")
	require.Greater(t, perfStats.MemInfo.MemPercent, float32(0.0), "expected mem percent to be set")
	require.Greater(t, perfStats.MemInfo.HeapTotal, uint64(0), "expected heap total to be set")
	require.Greater(t, perfStats.MemInfo.GoMemUsage, uint64(0), "expected go mem usage to be set")
	require.Greater(t, perfStats.MemInfo.NonGoMemUsage, uint64(0), "expected non go mem usage to be set")
}
