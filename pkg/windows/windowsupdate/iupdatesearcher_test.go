//go:build windows
// +build windows

package windowsupdate

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/scjalliance/comshim"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	startingGoMemoryUsage, startingNonGoMemoryUsage := checkMemoryUsage(t)

	// Set up update session + searcher
	comshim.Add(1)
	defer comshim.Done()
	session, err := NewUpdateSession()
	require.NoError(t, err)
	searcher, err := session.CreateUpdateSearcher()
	require.NoError(t, err)

	// Call `Search`
	results, err := searcher.Search("Type='Software'")
	require.NoError(t, err)
	require.NotNil(t, results)

	// Wait a short bit, then check memory usage again
	time.Sleep(5 * time.Second)
	endingGoMemoryUsage, endingNonGoMemoryUsage := checkMemoryUsage(t)

	// We expect that the memory will go up a bit, but we don't want it to
	// go up by more than a factor of 5.
	maxExpectedGoMemoryUsage := startingGoMemoryUsage * 5
	maxExpectedNonGoMemoryUsage := startingNonGoMemoryUsage * 5
	require.Less(t, endingGoMemoryUsage, maxExpectedGoMemoryUsage)
	require.Less(t, endingNonGoMemoryUsage, maxExpectedNonGoMemoryUsage)
}

// checkMemoryUsage pulls the current go and non-go memory usage for this
// process.
func checkMemoryUsage(t *testing.T) (uint64, uint64) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	currentGoMemory := stats.Sys - stats.HeapReleased

	currentPid := os.Getpid()
	currentProcess, err := process.NewProcess(int32(currentPid))
	require.NoError(t, err)
	memInfo, err := currentProcess.MemoryInfo()
	require.NoError(t, err)
	currentNonGoMemory := memInfo.RSS - currentGoMemory

	return currentGoMemory, currentNonGoMemory
}
