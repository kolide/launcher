//go:build windows
// +build windows

package windowsupdate

import (
	"testing"

	comshim "github.com/NozomiNetworks/go-comshim"
	"github.com/kolide/launcher/v2/ee/tables/ci"
	"github.com/stretchr/testify/require"
)

func initCOMBench(b *testing.B) {
	b.Helper()
	require.NoError(b, comshim.TryAdd(1), "initializing COM")
	b.Cleanup(comshim.Done)
}

// BenchmarkQueryHistory exercises the full COM lifecycle path in a loop:
// session creation, searcher creation, history query with real VARIANT
// extraction and IDispatch Release. The non-golang-B/op metric captures
// native memory growth -- this is where COM leaks from missing
// Release()/Clear() calls would show up.
func BenchmarkQueryHistory(b *testing.B) {
	initCOMBench(b)

	// Verify there's history to query; skip if not.
	session, err := NewUpdateSession()
	require.NoError(b, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(b, err)
	totalCount, err := searcher.GetTotalHistoryCount()
	require.NoError(b, err)

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
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}
