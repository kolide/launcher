//go:build darwin
// +build darwin

package appicons

import (
	"context"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkAppIcons(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up table
	appIconsTable := AppIcons(mockFlags, slogger)

	// Report memory allocations
	b.ReportAllocs()

	for range b.N {
		// Confirm we can call the table successfully
		response := appIconsTable.Call(context.TODO(), map[string]string{
			"action": "generate",
			"context": `{
	"constraints": [
		{
			"name": "path",
			"list": [
				{
					"op": 2,
					"expr": "not a real path"
				}
			]
		}
	]
}`,
		})

		// Briefly query worked
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}
