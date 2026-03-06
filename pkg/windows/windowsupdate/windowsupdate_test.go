//go:build windows
// +build windows

package windowsupdate

import (
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/ci"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BenchmarkQueryHistory exercises the full COM lifecycle path in a loop:
// session creation, searcher creation, history query with real VARIANT
// extraction and IDispatch Release. The non-golang-B/op metric captures
// native memory growth -- this is where COM leaks from missing
// Release()/Clear() calls would show up. The test fails if per-op
// native growth exceeds the threshold.
func BenchmarkQueryHistory(b *testing.B) {
	initCOMBench(b)

	// Verify there's history to query; skip if not.
	session, err := NewUpdateSession()
	require.NoError(b, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(b, err)
	totalCount, err := searcher.GetTotalHistoryCount()
	require.NoError(b, err)
	session.Release()

	if totalCount == 0 {
		b.Skip("no update history entries on this machine")
	}

	queryCount := totalCount
	if queryCount > 5 {
		queryCount = 5
	}

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		session, err := NewUpdateSession()
		require.NoError(b, err)

		searcher, err := session.CreateUpdateSearcher()
		require.NoError(b, err)

		entries, err := searcher.QueryHistory(0, queryCount)
		require.NoError(b, err)
		require.NotEmpty(b, entries)

		session.Release()
	}

	// 64 KiB per op is generous — a leaking implementation easily exceeds
	// this after the benchmark framework scales up b.N.
	ci.RequireNonGolangMemoryBelowThreshold(b, baselineStats, 64*1024)
}
