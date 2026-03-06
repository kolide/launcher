package ci

import (
	"testing"

	"github.com/kolide/launcher/v2/ee/performance"
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
	var nonGolangMemDifferenceInBytes uint64 = 0
	if statsAfter.MemInfo.NonGoMemUsage > baselineStats.MemInfo.NonGoMemUsage {
		nonGolangMemDifferenceInBytes = statsAfter.MemInfo.NonGoMemUsage - baselineStats.MemInfo.NonGoMemUsage
	}

	b.ReportMetric(float64(nonGolangMemDifferenceInBytes)/float64(b.N), "non-golang-B/op")
}

// RequireNonGolangMemoryBelowThreshold reports the non-Go memory metric and
// fails the benchmark if the per-operation growth exceeds maxBytesPerOp. This
// is useful for catching native memory leaks (e.g. COM IUnknown/VARIANT leaks)
// that live outside Go's garbage collector.
func RequireNonGolangMemoryBelowThreshold(b *testing.B, baselineStats *performance.PerformanceStats, maxBytesPerOp uint64) {
	b.Helper()

	statsAfter, err := performance.CurrentProcessStats(b.Context())
	require.NoError(b, err)
	var nonGolangMemDifferenceInBytes uint64 = 0
	if statsAfter.MemInfo.NonGoMemUsage > baselineStats.MemInfo.NonGoMemUsage {
		nonGolangMemDifferenceInBytes = statsAfter.MemInfo.NonGoMemUsage - baselineStats.MemInfo.NonGoMemUsage
	}

	perOp := nonGolangMemDifferenceInBytes / uint64(b.N)
	b.ReportMetric(float64(perOp), "non-golang-B/op")

	require.LessOrEqual(b, perOp, maxBytesPerOp,
		"non-Go memory grew %d B/op (total %d B over %d iterations), exceeding threshold of %d B/op — possible native memory leak",
		perOp, nonGolangMemDifferenceInBytes, b.N, maxBytesPerOp,
	)
}
