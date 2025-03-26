//go:build windows
// +build windows

package table

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/dsim_default_associations"
	"github.com/kolide/launcher/ee/tables/execparsers/dsregcmd"
	"github.com/kolide/launcher/ee/tables/secedit"
	"github.com/kolide/launcher/ee/tables/wifi_networks"
	"github.com/kolide/launcher/ee/tables/windowsupdatetable"
	"github.com/kolide/launcher/ee/tables/wmitable"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkProgramIcons(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	programIconsTable := ProgramIcons(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := programIconsTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkDsimDefaultAssocations(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	dsimDefaultAssociationsTable := dsim_default_associations.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := dsimDefaultAssociationsTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkSeceditTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	seceditTable := secedit.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := seceditTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWifiNetworksTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	wifiNetworksTable := wifi_networks.TablePlugin(mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := wifiNetworksTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWindowsUpdatesTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	updatesTable := windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := updatesTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWindowsHistoryTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	historyTable := windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, mockFlags, slogger)

	for range b.N {
		// Confirm we can call the table successfully
		response := historyTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkWmiTable(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	wmiTable := wmitable.TablePlugin(mockFlags, slogger)

	constraintsMap := map[string]any{
		"constraints": []map[string]any{
			{
				"name":     "class",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "SoftwareLicensingProduct",
					},
				},
			},
			{
				"name":     "properties",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "name,licensefamily,id,licensestatus,licensestatusreason,genuinestatus,partialproductkey,productkeyid",
					},
				},
			},
		},
	}
	queryContextStr, err := json.Marshal(constraintsMap)
	require.NoError(b, err)

	for range b.N {
		// Confirm we can call the table successfully
		response := wmiTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": string(queryContextStr),
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func BenchmarkDsregcmd(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	dsregcmdTable := dataflattentable.NewExecAndParseTable(mockFlags, slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`})

	for range b.N {
		// Confirm we can call the table successfully
		response := dsregcmdTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}

func TestMemoryUsage(t *testing.T) { //nolint:paralleltest
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up WMI query
	constraintsMap := map[string]any{
		"constraints": []map[string]any{
			{
				"name":     "class",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "SoftwareLicensingProduct",
					},
				},
			},
			{
				"name":     "properties",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "name,licensefamily,id,licensestatus,licensestatusreason,genuinestatus,partialproductkey,productkeyid",
					},
				},
			},
		},
	}
	queryContext, err := json.Marshal(constraintsMap)
	require.NoError(t, err)

	// Collect memstats before
	var statsBeforeAllTestCases runtime.MemStats
	runtime.ReadMemStats(&statsBeforeAllTestCases)
	heapTotalBeforeAllTestCases := heapTotal(&statsBeforeAllTestCases)
	goMemoryBeforeAllTestCases := goMemoryUsage(&statsBeforeAllTestCases)
	nonGoMemoryBeforeAllTestCases := nonGoMemoryUsage(t, &statsBeforeAllTestCases)

	for _, tt := range []struct {
		testCaseName string
		kolideTable  *table.Plugin
		queryContext string
	}{
		{
			testCaseName: "kolide_program_icons",
			kolideTable:  ProgramIcons(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_dsim_default_associations",
			kolideTable:  dsim_default_associations.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_secedit",
			kolideTable:  secedit.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_wifi_networks",
			kolideTable:  wifi_networks.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_windows_updates",
			kolideTable:  windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_windows_update_history",
			kolideTable:  windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_wmi",
			kolideTable:  wmitable.TablePlugin(mockFlags, slogger),
			queryContext: string(queryContext),
		},
		{
			testCaseName: "kolide_dsregcmd",
			kolideTable:  dataflattentable.NewExecAndParseTable(mockFlags, slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
			queryContext: "{}",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 20; i++ {
				callTable(t, tt.kolideTable, tt.queryContext)
			}
		})
	}

	time.Sleep(5 * time.Second)

	// Collect memstats after
	var statsAfterAllTestCases runtime.MemStats
	runtime.ReadMemStats(&statsAfterAllTestCases)

	heapTotalAfterAllTestCases := heapTotal(&statsAfterAllTestCases)
	goMemoryAfterAllTestCases := goMemoryUsage(&statsAfterAllTestCases)
	nonGoMemoryAfterAlltestCases := nonGoMemoryUsage(t, &statsAfterAllTestCases)

	fmt.Println("Heap total:")
	fmt.Printf("Before: %d\tAfter: %d\tDiff: %d\n", heapTotalBeforeAllTestCases, heapTotalAfterAllTestCases, heapTotalAfterAllTestCases-heapTotalBeforeAllTestCases)
	fmt.Println("Go memory:")
	fmt.Printf("Before: %d\tAfter: %d\tDiff: %d\n", goMemoryBeforeAllTestCases, goMemoryAfterAllTestCases, goMemoryAfterAllTestCases-goMemoryBeforeAllTestCases)
	fmt.Println("Non-go memory:")
	fmt.Printf("Before: %d\tAfter: %d\tDiff: %d\n", nonGoMemoryBeforeAllTestCases, nonGoMemoryAfterAlltestCases, nonGoMemoryAfterAlltestCases-nonGoMemoryBeforeAllTestCases)

	/*
		fmt.Println("Cumulative:")
		fmt.Printf("HeapTotal diff: %d\n", heapTotal(&statsAfterAllTestCases)-heapTotalBeforeAllTestCases)
		fmt.Printf("Alloc diff: %d\n", statsAfterAllTestCases.Alloc-statsBeforeAllTestCases.Alloc)
		fmt.Printf("Sys diff: %d\n", statsAfterAllTestCases.Sys-statsBeforeAllTestCases.Sys)
		fmt.Printf("Live objects diff: %d\n", (statsAfterAllTestCases.Mallocs-statsAfterAllTestCases.Frees)-(statsBeforeAllTestCases.Mallocs-statsBeforeAllTestCases.Frees))
		fmt.Printf("HeapAlloc diff: %d\n", statsAfterAllTestCases.HeapAlloc-statsBeforeAllTestCases.HeapAlloc)
		fmt.Printf("HeapIdle diff: %d\n", statsAfterAllTestCases.HeapIdle-statsBeforeAllTestCases.HeapIdle)
		fmt.Printf("HeapInuse diff: %d\n", statsAfterAllTestCases.HeapInuse-statsBeforeAllTestCases.HeapInuse)
		fmt.Printf("HeapObjects diff: %d\n", statsAfterAllTestCases.HeapObjects-statsBeforeAllTestCases.HeapObjects)
	*/
}

func callTable(t *testing.T, kolideTable *table.Plugin, queryContext string) {
	// Confirm we can call the table successfully
	response := kolideTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": queryContext,
	})

	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
}

func heapTotal(m *runtime.MemStats) uint64 {
	bytesAllocatedToObjects := m.HeapAlloc // both live and dead
	freeBytes := m.HeapIdle - m.HeapReleased
	unusedBytes := m.HeapInuse - m.HeapAlloc

	return bytesAllocatedToObjects + freeBytes + unusedBytes
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

func TestMemoryUsageWithMemprofile(t *testing.T) { //nolint:paralleltest
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up WMI query
	constraintsMap := map[string]any{
		"constraints": []map[string]any{
			{
				"name":     "class",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "SoftwareLicensingProduct",
					},
				},
			},
			{
				"name":     "properties",
				"affinity": "TEXT",
				"list": []map[string]any{
					{
						"op":   "2", // table.OperatorEquals
						"expr": "name,licensefamily,id,licensestatus,licensestatusreason,genuinestatus,partialproductkey,productkeyid",
					},
				},
			},
		},
	}
	queryContext, err := json.Marshal(constraintsMap)
	require.NoError(t, err)

	// Collect memprofile before
	outBefore, err := os.Create("memprofile-before.out")
	require.NoError(t, err)
	require.NoError(t, pprof.WriteHeapProfile(outBefore))
	require.NoError(t, outBefore.Close())

	for _, tt := range []struct { //nolint:paralleltest
		testCaseName string
		kolideTable  *table.Plugin
		queryContext string
	}{
		{
			testCaseName: "kolide_program_icons",
			kolideTable:  ProgramIcons(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_dsim_default_associations",
			kolideTable:  dsim_default_associations.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_secedit",
			kolideTable:  secedit.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_wifi_networks",
			kolideTable:  wifi_networks.TablePlugin(mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_windows_updates",
			kolideTable:  windowsupdatetable.TablePlugin(windowsupdatetable.UpdatesTable, mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_windows_update_history",
			kolideTable:  windowsupdatetable.TablePlugin(windowsupdatetable.HistoryTable, mockFlags, slogger),
			queryContext: "{}",
		},
		{
			testCaseName: "kolide_wmi",
			kolideTable:  wmitable.TablePlugin(mockFlags, slogger),
			queryContext: string(queryContext),
		},
		{
			testCaseName: "kolide_dsregcmd",
			kolideTable:  dataflattentable.NewExecAndParseTable(mockFlags, slogger, "kolide_dsregcmd", dsregcmd.Parser, allowedcmd.Dsregcmd, []string{`/status`}),
			queryContext: "{}",
		},
	} { //nolint:paralleltest
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				callTable(t, tt.kolideTable, tt.queryContext)
			}
		})
	}

	// Collect memprofile before
	outAfter, err := os.Create("memprofile-after.out")
	require.NoError(t, err)
	require.NoError(t, pprof.WriteHeapProfile(outAfter))
	require.NoError(t, outAfter.Close())
}
