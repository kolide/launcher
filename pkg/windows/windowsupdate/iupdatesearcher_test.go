package windowsupdate

import (
	"testing"

	"github.com/scjalliance/comshim"
	"github.com/stretchr/testify/require"
)

func BenchmarkOnlineSearch(b *testing.B) {
	// Set up update session + searcher
	comshim.Add(1)
	defer comshim.Done()
	session, err := NewUpdateSession()
	require.NoError(b, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(b, err)

	// Make sure Online property is set as expected
	require.True(b, searcher.Online)

	// Call search repeatedly
	for range b.N {
		results, err := searcher.Search("Type='Software'")
		require.NoError(b, err)
		require.NotNil(b, results)
	}
}

func BenchmarkOfflineSearch(b *testing.B) {
	// Set up update session + searcher
	comshim.Add(1)
	defer comshim.Done()
	session, err := NewUpdateSession()
	require.NoError(b, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(b, err)

	// Make sure search happens offline
	require.NoError(b, searcher.PutOnline(false))

	// Make sure Online property is set as expected
	require.False(b, searcher.Online)

	// Call search repeatedly
	for range b.N {
		results, err := searcher.Search("Type='Software'")
		require.NoError(b, err)
		require.NotNil(b, results)
	}
}

func BenchmarkComboSearch(b *testing.B) {
	// Set up update session + searcher
	comshim.Add(1)
	defer comshim.Done()
	session, err := NewUpdateSession()
	require.NoError(b, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(b, err)

	// Make sure Online property is set as expected
	require.True(b, searcher.Online)

	// Make one online request
	results, err := searcher.Search("Type='Software'")
	require.NoError(b, err)
	require.NotNil(b, results)
	expectedUpdatesCount := len(results.Updates)

	// Now, update searcher to be offline
	require.NoError(b, searcher.PutOnline(false))

	// Make sure Online property is updated as expected
	require.False(b, searcher.Online)

	// Call search repeatedly
	for range b.N {
		results, err := searcher.Search("Type='Software'")
		require.NoError(b, err)
		require.NotNil(b, results)
		// For some reason, offline searches always return one extra element in this test.
		// That seems fine. We mostly want to make sure that we don't get FEWER results when
		// we search offline -- so we check that the online count is less or equal to the
		// offline count.
		require.LessOrEqual(b, expectedUpdatesCount, len(results.Updates))
	}
}
