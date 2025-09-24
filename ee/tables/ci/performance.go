//go:build performance
// +build performance

package ci

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/performance"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

const rssMax uint64 = 10 * 1024 * 1024 // 10 MB

// AssessMemoryImpact runs the given query in `queryContext` the number of times specified in `queryCount`
// against the given table, and assesses the difference in memory usage before and after running. Because
// we are assessing memory for the current process, AssessMemoryImpact should only be called by a test running
// standalone, not in parallel with any other tests, to avoid poisoning the results.
// If `expectQuerySuccess` is true, AssessMemoryImpact will additionally confirm that the query is successful.
// Generally, we want to know that the query did actually run successfully -- but for some tables, failure
// will be unavoidable, so we provide the option here.
func AssessMemoryImpact(t *testing.T, testTable *table.Plugin, queryContext string, queryCount int, expectQuerySuccess bool) {
	// Tests panic after 10 minutes, so set a 9-minute timeout just to be safe
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	// Establish baseline memory usage
	statsBefore, err := performance.CurrentProcessStats(ctx)
	require.NoError(t, err)

	// Perform the required number of queries
	for range queryCount {
		response := testTable.Call(ctx, map[string]string{
			"action":  "generate",
			"context": queryContext,
		})

		if expectQuerySuccess {
			require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
		}
	}

	// Sleep for a couple seconds
	time.Sleep(3 * time.Second)

	// Measure memory usage after performing queries
	statsAfter, err := performance.CurrentProcessStats(ctx)
	require.NoError(t, err)

	// Determine how much memory was allocated to running tests that has not yet been collected
	rssDifferenceInBytes := statsAfter.MemInfo.RSS - statsBefore.MemInfo.RSS
	golangMemDifferenceInBytes := statsAfter.MemInfo.GoMemUsage - statsBefore.MemInfo.GoMemUsage
	nonGolangMemDifferenceInBytes := statsAfter.MemInfo.NonGoMemUsage - statsBefore.MemInfo.NonGoMemUsage

	// Check that we're under the threshold
	require.Less(t, rssDifferenceInBytes, rssMax, fmt.Sprintf("unexpectedly high memory usage by %s: RSS difference: %d MB; Golang difference: %d MB; non-Golang difference: %d MB", testTable.Name(), rssDifferenceInBytes/(1024*1024), golangMemDifferenceInBytes/(1024*1024), nonGolangMemDifferenceInBytes/(1024*1024)))

	// TODO RM -- just nice to see while testing
	fmt.Printf("RSS difference: %d MB; Golang difference: %d MB; non-Golang difference: %d MB\n", rssDifferenceInBytes/(1024*1024), golangMemDifferenceInBytes/(1024*1024), nonGolangMemDifferenceInBytes/(1024*1024))
}
