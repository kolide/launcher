package ci

import (
	"testing"

	"github.com/kolide/launcher/ee/performance"
	"github.com/stretchr/testify/require"
)

func BaselineStats(b *testing.B) *performance.PerformanceStats {
	baselineStats, err := performance.CurrentProcessStats(b.Context())
	require.NoError(b, err)
	return baselineStats
}

func ReportNonGolangMemoryUsage(b *testing.B, baselineStats *performance.PerformanceStats) {
	statsAfter, err := performance.CurrentProcessStats(b.Context())
	require.NoError(b, err)
	nonGolangMemDifferenceInBytes := statsAfter.MemInfo.NonGoMemUsage - baselineStats.MemInfo.NonGoMemUsage

	b.ReportMetric(float64(nonGolangMemDifferenceInBytes)/float64(b.N), "non-golang-B/op")
}
