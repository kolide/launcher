//go:build windows
// +build windows

package dsim_default_associations

import (
	"context"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func BenchmarkDsimDefaultAssocations(b *testing.B) {
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	dsimDefaultAssociationsTable := TablePlugin(mockFlags, slogger)

	for b.Loop() {
		// Confirm we can call the table successfully
		response := dsimDefaultAssociationsTable.Call(context.TODO(), map[string]string{
			"action":  "generate",
			"context": "{}",
		})

		require.Equal(b, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	}
}
