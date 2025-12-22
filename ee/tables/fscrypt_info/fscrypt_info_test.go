//go:build linux

package fscrypt_info

import (
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/tables/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkFscryptInfo(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up table
	fscryptTable := TablePlugin(mockFlags, slogger)

	// Report memory allocations
	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()

	for range b.N {
		// The table requires at least one path constraint.
		response := fscryptTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("path", "/"))

		// Briefly confirm query worked
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}
