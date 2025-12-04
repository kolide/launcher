//go:build windows
// +build windows

package dsim_default_associations

import (
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkDismDefaultAssociations(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up table
	dsimTable := TablePlugin(mockFlags, slogger)

	// Report memory allocations
	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()

	for range b.N {
		// Confirm we can call the table successfully
		response := dsimTable.Call(b.Context(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		// Briefly confirm query worked
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}
