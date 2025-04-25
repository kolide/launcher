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

	// Make one online request
	results, err := searcher.Search("Type='Software'")
	require.NoError(b, err)
	require.NotNil(b, results)
	expectedUpdatesCount := len(results.Updates)

	// Now, update searcher to be offline
	require.NoError(b, searcher.PutOnline(false))

	// Call search repeatedly
	for range b.N {
		results, err := searcher.Search("Type='Software'")
		require.NoError(b, err)
		require.NotNil(b, results)
		require.Equal(b, expectedUpdatesCount, len(results.Updates))
	}
}
