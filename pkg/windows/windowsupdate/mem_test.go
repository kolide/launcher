package windowsupdate

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/go-ole/go-ole/oleutil"
	"github.com/kolide/launcher/pkg/windows/oleconv"
	"github.com/scjalliance/comshim"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/require"
)

func TestMemory(t *testing.T) {
	fmt.Println("starting")
	takeAndPrintMemoryUsageSnapshot(t)

	comshim.Add(1)
	defer comshim.Done()

	session, err := NewUpdateSession()
	require.NoError(t, err)

	fmt.Println("created update session")
	takeAndPrintMemoryUsageSnapshot(t)

	searcher, err := session.CreateUpdateSearcher()
	require.NoError(t, err)

	fmt.Println("created update searcher")
	takeAndPrintMemoryUsageSnapshot(t)

	searchVariant, err := oleutil.CallMethod(searcher.disp, "Search", "Type='Software'")
	require.NoError(t, err)

	fmt.Println("after Search")
	takeAndPrintMemoryUsageSnapshot(t)

	searchResultDisp, err := oleconv.ToIDispatchErr(searchVariant, err)
	require.NoError(t, err)

	fmt.Println("converted to IDispatch")
	takeAndPrintMemoryUsageSnapshot(t)

	_, err = toISearchResult(searchResultDisp)
	require.NoError(t, err)

	fmt.Println("converted to ISearchResult")
	takeAndPrintMemoryUsageSnapshot(t)

	searcher.disp.Release()
	session.disp.Release()
	require.NoError(t, searchVariant.Clear())

	time.Sleep(10 * time.Second)

	fmt.Println("after releasing searcher and session + clearing search variant")
	takeAndPrintMemoryUsageSnapshot(t)
}

func takeAndPrintMemoryUsageSnapshot(t *testing.T) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	currentGoMemory := goMemoryUsage(&stats)
	currentNonGoMemory := nonGoMemoryUsage(t, &stats)

	fmt.Printf("Go memory: %d\tNon-go memory: %d\n", currentGoMemory, currentNonGoMemory)
}

// Non-Go memory usage (e.g. cgo) can maybe be estimated as Process RSS - Go memory
func nonGoMemoryUsage(t *testing.T, m *runtime.MemStats) uint64 {
	currentPid := os.Getpid()
	currentProcess, err := process.NewProcess(int32(currentPid))
	require.NoError(t, err)
	memInfo, err := currentProcess.MemoryInfo()
	require.NoError(t, err)
	return memInfo.RSS - goMemoryUsage(m)
}

/*
[T]he following expression accurately reflects the value the runtime attempts to maintain as the limit:

runtime.MemStats.Sys − runtime.MemStats.HeapReleased
*/
func goMemoryUsage(m *runtime.MemStats) uint64 {
	return m.Sys - m.HeapReleased
}
