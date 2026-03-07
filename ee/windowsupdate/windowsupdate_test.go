//go:build windows
// +build windows

package windowsupdate

import (
	"testing"

	comshim "github.com/NozomiNetworks/go-comshim"
	"github.com/kolide/launcher/v2/ee/tables/ci"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initCOM(t *testing.T) {
	t.Helper()
	require.NoError(t, comshim.TryAdd(1), "initializing COM")
	t.Cleanup(comshim.Done)
}

func initCOMBench(b *testing.B) {
	b.Helper()
	require.NoError(b, comshim.TryAdd(1), "initializing COM")
	b.Cleanup(comshim.Done)
}

func TestNewUpdateSession(t *testing.T) {
	t.Parallel()
	initCOM(t)

	session, err := NewUpdateSession()
	require.NoError(t, err, "NewUpdateSession")
	defer session.Release()

	// ClientApplicationID is a string (may be empty on a default session)
	assert.IsType(t, "", session.ClientApplicationID)
	// ReadOnly should be a bool; default sessions are not read-only
	assert.False(t, session.ReadOnly)
}

func TestCreateUpdateSearcher(t *testing.T) {
	t.Parallel()
	initCOM(t)

	session, err := NewUpdateSession()
	require.NoError(t, err, "NewUpdateSession")
	defer session.Release()

	searcher, err := session.CreateUpdateSearcher()
	require.NoError(t, err, "CreateUpdateSearcher")

	// ServerSelection is an enum: ssDefault(0), ssManagedServer(1), ssWindowsUpdate(2), ssOthers(3)
	assert.GreaterOrEqual(t, searcher.ServerSelection, int32(0))
	assert.LessOrEqual(t, searcher.ServerSelection, int32(3))

	assert.IsType(t, "", searcher.ServiceID)
}

func TestGetTotalHistoryCount(t *testing.T) {
	t.Parallel()
	initCOM(t)

	session, err := NewUpdateSession()
	require.NoError(t, err, "NewUpdateSession")
	defer session.Release()

	searcher, err := session.CreateUpdateSearcher()
	require.NoError(t, err, "CreateUpdateSearcher")

	count, err := searcher.GetTotalHistoryCount()
	require.NoError(t, err, "GetTotalHistoryCount")
	assert.GreaterOrEqual(t, count, int32(0), "history count should be non-negative")
}

func TestQueryHistorySmall(t *testing.T) {
	t.Parallel()
	initCOM(t)

	session, err := NewUpdateSession()
	require.NoError(t, err, "NewUpdateSession")
	defer session.Release()

	searcher, err := session.CreateUpdateSearcher()
	require.NoError(t, err, "CreateUpdateSearcher")

	totalCount, err := searcher.GetTotalHistoryCount()
	require.NoError(t, err, "GetTotalHistoryCount")

	if totalCount == 0 {
		t.Skip("no update history entries on this machine, skipping")
	}

	// Query a small number of entries to keep the test fast
	queryCount := totalCount
	if queryCount > 3 {
		queryCount = 3
	}

	entries, err := searcher.QueryHistory(0, queryCount)
	require.NoError(t, err, "QueryHistory")
	require.Len(t, entries, int(queryCount))

	for i, entry := range entries {
		assert.NotEmpty(t, entry.Title, "entry[%d].Title should not be empty", i)

		// OperationResultCode enum: orcNotStarted(0), orcInProgress(1), orcSucceeded(2),
		// orcSucceededWithErrors(3), orcFailed(4), orcAborted(5)
		assert.GreaterOrEqual(t, entry.ResultCode, int32(0), "entry[%d].ResultCode", i)
		assert.LessOrEqual(t, entry.ResultCode, int32(5), "entry[%d].ResultCode", i)

		// UpdateOperation enum: uoInstallation(1), uoUninstallation(2)
		assert.GreaterOrEqual(t, entry.Operation, int32(1), "entry[%d].Operation", i)
		assert.LessOrEqual(t, entry.Operation, int32(2), "entry[%d].Operation", i)

		// UpdateIdentity should be populated
		if assert.NotNil(t, entry.UpdateIdentity, "entry[%d].UpdateIdentity", i) {
			assert.NotEmpty(t, entry.UpdateIdentity.UpdateID, "entry[%d].UpdateIdentity.UpdateID", i)
		}
	}
}

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

	ci.ReportNonGolangMemoryUsage(b, baselineStats)

	// 64 KiB per op is generous — a leaking implementation easily exceeds
	// this after the benchmark framework scales up b.N.
	ci.RequireNonGolangMemoryBelowThreshold(b, baselineStats, 64*1024)
}
